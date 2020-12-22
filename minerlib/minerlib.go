package minerlib

import (
	"github.com/cryptonote-social/csminer/blockchain"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/minerlib/stats"
	"github.com/cryptonote-social/csminer/rx"
	"github.com/cryptonote-social/csminer/stratum/client"

	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// Indicates there is no connection to the pool server, either because there has yet to
	// be a successful login, or there are connectivity issues. For the latter case, the
	// miner will continue trying to connect.
	MINING_PAUSED_NO_CONNECTION = -2

	// Indicates miner is paused because the screen is active
	MINING_PAUSED_SCREEN_ACTIVITY = -3

	// Indicates miner is paused because the machine is operating on battery power.
	MINING_PAUSED_BATTERY_POWER = -4

	// Indicates miner is paused, and is in the "user focred mining pause" state.
	MINING_PAUSED_USER_OVERRIDE = -5

	// Indicates miner is paused because we're in the user-excluded time period
	MINING_PAUSED_TIME_EXCLUDED = -6

	// Indicates the most recent login failed so there is no connection to the pool
	// server. Prompt the user to log in with valid log in parameters.
	MINING_PAUSED_NO_LOGIN = -7

	// Indicates miner is actively mining
	MINING_ACTIVE = 1

	// Indicates miner is actively mining due to user-initiated override
	MINING_ACTIVE_USER_OVERRIDE = 2

	// for PokeChannel stuff:
	HANDLED    = 1
	USE_CACHED = 2

	STATE_CHANGE_POKE     = 1
	INCREASE_THREADS_POKE = 6
	DECREASE_THREADS_POKE = 7
	EXIT_LOOP_POKE        = 8
	UPDATE_STATS_POKE     = 9

	OVERRIDE_MINE  = 1
	OVERRIDE_PAUSE = 2
)

var (
	// miner config
	configMutex sync.Mutex
	// plArgs (pool login args) is nil if nobody is currently logged in, which also implies
	// dispatch loop isn't active.
	plArgs                           *PoolLoginArgs
	threads                          int
	lastSeed                         []byte
	excludeHourStart, excludeHourEnd int

	doneChanMutex      sync.Mutex
	miningLoopDoneChan chan bool // non-nil when a mining loop is active

	batteryPower   bool
	screenIdle     bool
	miningOverride int // 0 == no override, OVERRIDE_MINE == always mine, OVERRIDE_PAUSE == don't mine

	// stratum client
	cl client.Client

	// used to send messages to main job loop to take various actions
	pokeChannel chan int

	// Worker thread synchronization vars
	wg      sync.WaitGroup // used to wait for stopped worker threads to finish
	stopper uint32         // atomic int used to signal rxlib worker threads to stop mining
)

type PoolLoginArgs struct {
	// username: a properly formatted pool username.
	Username string

	// rigid: a properly formatted rig id, or null if no rigid is specified by the user.
	RigID string

	// wallet: a properly formatted wallet address; can be null for username-only logins. If wallet is
	//         null, pool server will return a warning if the username has not previously been
	//         associated with a wallet.
	Wallet string

	// agent: a string that informs the pool server of the miner client details, e.g. name and version
	//        of the software using this API.
	Agent string

	// config: advanced options config string, can be null.
	Config string

	// UseTLS: Whether to use TLS when connecting to the pool
	UseTLS bool
}

type PoolLoginResponse struct {
	// code = 1: login successful; if message is non-null, it's a warning/info message from pool
	//           server that should be shown to the user
	//
	// code < 0: login unsuccessful; couldn't reach pool server. Caller should retry later. message
	//           will contain the connection-level error encountered.
	//
	// code > 1: login unsuccessful; pool server refused login. Message will contain information that
	//           can be shown to user to help fix the problem. Caller should retry with new login
	//           parameters.
	Code      int
	Message   string
	MessageID int
}

// See MINING_ACTIVITY const values above for all possibilities. Shorter story: negative value ==
// paused, posiive value == active.
func getMiningActivityState() int {
	configMutex.Lock()
	defer configMutex.Unlock()

	if plArgs == nil {
		return MINING_PAUSED_NO_LOGIN
	}

	// User-override pause trumps all:
	if miningOverride == OVERRIDE_PAUSE {
		return MINING_PAUSED_USER_OVERRIDE
	}
	// If there is no pool connection, we cannot mine.
	if !cl.IsAlive() {
		return MINING_PAUSED_NO_CONNECTION
	}

	if miningOverride == OVERRIDE_MINE {
		return MINING_ACTIVE_USER_OVERRIDE
	}

	if timeExcluded() {
		return MINING_PAUSED_TIME_EXCLUDED
	}

	if batteryPower {
		return MINING_PAUSED_BATTERY_POWER
	}
	if !screenIdle {
		return MINING_PAUSED_SCREEN_ACTIVITY
	}

	return MINING_ACTIVE
}

// Called by the user to log into the pool for the first time, or re-log into the pool with new
// credentials.
func PoolLogin(args *PoolLoginArgs) *PoolLoginResponse {
	crylog.Info("Pool login called")
	doneChanMutex.Lock()
	defer doneChanMutex.Unlock()
	if miningLoopDoneChan != nil {
		crylog.Info("Pool login: shutting down previous mining loop")
		// trigger close of previous mining loop
		pokeJobDispatcher(EXIT_LOOP_POKE)
		// wait until previous mining loop completes
		<-miningLoopDoneChan
		miningLoopDoneChan = nil
		crylog.Info("Pool login: Previous loop done")
	}

	configMutex.Lock()
	defer configMutex.Unlock()
	plArgs = nil
	r := &PoolLoginResponse{}
	loginName := args.Username
	if strings.Index(args.Username, ".") != -1 {
		// Handle this specially since xmrig style login might cause users to specify wallet.username here
		r.Code = 2
		r.Message = "The '.' character is not allowed in usernames."
		return r
	}
	if args.Wallet != "" {
		loginName = args.Wallet + "." + args.Username
	}
	agent := args.Agent
	config := args.Config
	rigid := args.RigID

	err, code, message, jc := cl.Connect("cryptonote.social:5555", args.UseTLS, agent, loginName, config, rigid)
	if err != nil {
		if code != 0 {
			//crylog.Error("Pool server did not allow login due to error:")
			//crylog.Error("  ::::::", message, "::::::")
			r.Code = 2
			r.Message = message
			return r
		}
		//crylog.Error("Couldn't connect to pool server:", err)
		r.Code = -1
		r.Message = err.Error()
		return r
	} else if code != 0 {
		// We got a warning from the stratum server
		//crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::\n")
		//if code == client.NO_WALLET_SPECIFIED_WARNING_CODE {
		//crylog.Warn("WARNING: your username is not yet associated with any")
		//crylog.Warn("   wallet id. You should fix this immediately.")
		//} else {
		//crylog.Warn("WARNING from pool server")
		//crylog.Warn("   Message:", message)
		//}
		//crylog.Warn("   Code   :", code, "\n")
		//crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::\n")
		r.MessageID = code
		r.Message = message
	}

	// login successful
	plArgs = args
	r.Code = 1
	go stats.RefreshPoolStats(plArgs.Username)
	miningLoopDoneChan = make(chan bool, 1)
	go MiningLoop(jc, miningLoopDoneChan)
	crylog.Info("Successful login:", plArgs.Username)
	return r
}

type InitMinerArgs struct {
	// threads specifies the initial # of threads to mine with. Must be >=1
	Threads int

	// begin/end hours (24 time) of the time during the day where mining should be paused. Set both
	// to 0 if there is no excluded range.
	ExcludeHourStart, ExcludeHourEnd int
}

type InitMinerResponse struct {
	// code == 1: miner init successful
	//
	// code == 2: miner init successful but hugepages could not be enabled, so mining may be
	//            slow. You can suggest to the user that a machine restart might help resolve this.
	//
	// code > 2: miner init failed due to bad config, see details in message. For example, an
	//           invalid number of threads or invalid hour range may have been specified.
	//
	// code < 0: non-recoverable error, message will provide details. program should exit after
	//           showing message.
	Code    int
	Message string
}

// InitMiner configures the miner and must be called exactly once before any other method
// is called.
func InitMiner(args *InitMinerArgs) *InitMinerResponse {
	pokeChannel = make(chan int, 5) // use small amount of buffering for when internet may be bad
	r := &InitMinerResponse{}
	hr1 := args.ExcludeHourStart
	hr2 := args.ExcludeHourEnd
	if hr1 > 24 || hr1 < 0 || hr2 > 24 || hr2 < 0 {
		r.Code = 3
		r.Message = "exclude_hour_start and exclude_hour_end must each be between 0 and 24"
		return r
	}
	excludeHourStart = hr1
	excludeHourEnd = hr2

	code := rx.InitRX(args.Threads)
	if code < 0 {
		crylog.Error("Failed to initialize RandomX")
		r.Code = -3
		r.Message = "Failed to initialize RandomX"
		return r
	}
	if code == 2 {
		r.Code = 2
	} else {
		r.Code = 1
	}
	stats.Init()
	threads = args.Threads
	crylog.Info("minerlib initialized")
	return r

}

// Returns nil if connection could not be established, in which case caller should make sure mining
// loop isn't supposed to terminate, and otherwise try again after a brief sleep. On success, returns
// a new job channel on which to continue listening for jobs.
func reconnectClient() <-chan *client.MultiClientJob {
	configMutex.Lock()
	defer configMutex.Unlock()

	var err error
	if plArgs == nil {
		err = errors.New("plArgs was nil")
		return nil
	}
	loginName := plArgs.Username
	if plArgs.Wallet != "" {
		loginName = plArgs.Wallet + "." + plArgs.Username
	}
	crylog.Info("Attempting to reconnect...")
	err, code, message, jc := cl.Connect(
		"cryptonote.social:5555", plArgs.UseTLS, plArgs.Agent, loginName, plArgs.Config, plArgs.RigID)
	if err == nil {
		if code != 0 {
			crylog.Warn("Pool server returned login warning:", message)
		}
		return jc
	}
	crylog.Error("Connect to pool server failed:", err)
	if code != 0 {
		crylog.Error("Pool server did not allow login due to error:", message)
	}
	return nil
}

// Called by PoolLogin after succesful login.
func MiningLoop(jobChan <-chan *client.MultiClientJob, done chan<- bool) {
	defer func() { done <- true }()

	// Set up fresh stats ....
	stopWorkers()
	stats.ResetRecent()

	lastActivityState := -999
	var job *client.MultiClientJob
	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		select {
		case poke := <-pokeChannel:
			if poke == EXIT_LOOP_POKE {
				crylog.Info("Stopping mining loop")
				stopWorkers()
				return
			}
			handlePoke(poke)
			if job == nil {
				crylog.Warn("no job to work on")
				continue
			}

		case job = <-jobChan:
			if job == nil {
				crylog.Info("stratum client closed, reconnecting...")
				cl.Close()
				newChan := reconnectClient()
				if newChan == nil {
					crylog.Info("reconnect failed. sleeping for", sleepSec, "seconds before trying again")
					time.Sleep(sleepSec)
					sleepSec += time.Second
					continue
				}
				// Set up fresh stats for new connection
				stopWorkers()
				stats.ResetRecent()
				sleepSec = 3 * time.Second
				jobChan = newChan
				continue
			}

			infoStr := fmt.Sprint("Current job: ", job.JobID, "  Difficulty: ", blockchain.TargetToDifficulty(job.Target))
			if getMiningActivityState() < 0 {
				crylog.Info(infoStr, " Mining: PAUSED")
			} else {
				crylog.Info(infoStr, " Mining: ACTIVE")
			}

		case <-time.After(30 * time.Second):
			break
		}

		stopWorkers()

		// Check if we need to reinitialize rx dataset
		newSeed, err := hex.DecodeString(job.SeedHash)
		if err != nil {
			crylog.Error("invalid seed hash:", job.SeedHash)
			continue
		}
		if bytes.Compare(newSeed, lastSeed) != 0 {
			crylog.Info("New seed:", job.SeedHash)
			rx.SeedRX(newSeed, runtime.GOMAXPROCS(0))
			lastSeed = newSeed
			stats.ResetRecent()
		}

		as := getMiningActivityState()
		if as != lastActivityState {
			crylog.Info("New activity state:", getActivityMessage(as))
			if (as < 0 && lastActivityState > 0) || (as > 0 && lastActivityState < 0) {
				stats.ResetRecent()
			}
			lastActivityState = as
		}
		if as < 0 {
			continue
		}

		atomic.StoreUint32(&stopper, 0)
		for i := 0; i < threads; i++ {
			wg.Add(1)
			go goMine(*job, i /*thread*/)
		}
	}
}

// Stop all active worker threads and wait for them to finish before returning. Should
// only be called by the MiningLoop.
func stopWorkers() {
	atomic.StoreUint32(&stopper, 1)
	wg.Wait()
	stats.RecentStatsNowAccurate()
}

func handlePoke(poke int) {
	switch poke {
	case INCREASE_THREADS_POKE:
		stopWorkers()
		configMutex.Lock()
		t := rx.AddThread()
		if t < 0 {
			configMutex.Unlock()
			crylog.Error("Failed to add another thread")
			return
		}
		threads = t
		configMutex.Unlock()
		crylog.Info("Increased # of threads to:", t)
		stats.ResetRecent()
		return

	case DECREASE_THREADS_POKE:
		stopWorkers()
		configMutex.Lock()
		t := rx.RemoveThread()
		if t < 0 {
			configMutex.Unlock()
			crylog.Error("Failed to decrease threads")
			return
		}
		threads = t
		configMutex.Unlock()
		crylog.Info("Decreased # of threads to:", t)
		stats.ResetRecent()
		return

	case STATE_CHANGE_POKE:
		stats.ResetRecent()
		return

	case UPDATE_STATS_POKE:
		return
	}
	crylog.Error("Unexpected poke:", poke)
}

type GetMiningStateResponse struct {
	stats.Snapshot
	MiningActivity int
	Threads        int
}

// poke the job dispatcher to refresh recent stats. result may not be immediate but should happen
// quickly.
func RequestRecentStatsUpdate() {
	configMutex.Lock()
	defer configMutex.Unlock()
	if plArgs == nil {
		// dispatch loop inactive so there are no stats to update
		return
	}
	go pokeJobDispatcher(UPDATE_STATS_POKE) // own gorouting so as not to block
}

func GetMiningState() *GetMiningStateResponse {
	as := getMiningActivityState()
	var isMining bool
	if as > 0 {
		isMining = true
	}
	s, _, _ := stats.GetSnapshot(isMining)
	configMutex.Lock()
	defer configMutex.Unlock()
	if plArgs == nil {
		s.PoolUsername = ""
		s.SecondsOld = -1.0
	} else if plArgs.Username != s.PoolUsername {
		// Pool stats do not (yet) reflect the currently logged in user, so tag them as invalid.
		s.PoolUsername = plArgs.Username
		s.SecondsOld = -1.0
	}
	return &GetMiningStateResponse{
		Snapshot:       *s,
		MiningActivity: as,
		Threads:        threads,
	}
}

func updatePoolStats(isMining bool) {
	s, _, _ := stats.GetSnapshot(isMining)
	configMutex.Lock()
	defer configMutex.Unlock()
	if plArgs == nil {
		return
	}
	uname := plArgs.Username
	if uname != "" && (uname != s.PoolUsername || s.SecondsOld > 5) {
		go stats.RefreshPoolStats(uname)
	}
}

func IncreaseThreads() {
	configMutex.Lock()
	defer configMutex.Unlock()
	if plArgs != nil {
		go pokeJobDispatcher(INCREASE_THREADS_POKE)
		return
	}
	// dispatch loop isn't active so just handle this here
	t := rx.AddThread()
	if t < 0 {
		configMutex.Unlock()
		crylog.Error("Failed to add another thread")
		return
	}
	threads = t
}

func DecreaseThreads() {
	configMutex.Lock()
	defer configMutex.Unlock()
	if plArgs != nil {
		go pokeJobDispatcher(DECREASE_THREADS_POKE)
		return
	}
	// dispatch loop isn't active so just handle this here
	t := rx.RemoveThread()
	if t < 0 {
		configMutex.Unlock()
		crylog.Error("Failed to decrease threads")
		return
	}
	threads = t
}

// Poke the job dispatcher. Though it should be unlikely, this method may block if the channel is
// full, so invoke it in a goroutine if you wish to never block.
func pokeJobDispatcher(pokeMsg int) {
	pokeChannel <- pokeMsg
}

/*
func printStats(isMining bool) {
	s, _, window := stats.GetSnapshot(isMining)
	configMutex.Lock()
	if disableStatsPrinting {
		configMutex.Unlock()
		return
	}
	crylog.Info("Recent hashrate computation window (seconds):", window)
	crylog.Info("=====================================")
	if s.RecentHashrate >= 0.0 {
		crylog.Info("Hashrate:", strconv.FormatFloat(s.RecentHashrate, 'f', 2, 64))
	} else {
		crylog.Info("Hashrate: --calculating--")
	}
	uname := plArgs.Username
	crylog.Info("Threads:", threads)
	configMutex.Unlock()
	if s.PoolUsername != "" && uname == s.PoolUsername {
		crylog.Info("== Pool stats last updated", s.SecondsOld, "seconds ago:")
		crylog.Info("User               :", s.PoolUsername)
		crylog.Info("Lifetime hashes    :", s.LifetimeHashes)
		crylog.Info("Paid               :", strconv.FormatFloat(s.Paid, 'f', 12, 64), "$XMR")
		if s.Owed > 0.0 {
			crylog.Info("Owed               :", strconv.FormatFloat(s.Owed, 'f', 12, 64), "$XMR")
		}
		crylog.Info("Accumulated        :", strconv.FormatFloat(s.Accumulated, 'f', 12, 64), "$XMR")
		crylog.Info("Time to next reward:", s.TimeToReward)
		if len(s.TimeToReward) > 0 {

		}
		if uname != s.PoolUsername || s.SecondsOld > 120 {
			go stats.RefreshPoolStats(uname)
		}
	}
	crylog.Info("=====================================")
}
*/

func goMine(job client.MultiClientJob, thread int) {
	defer wg.Done()
	input, err := hex.DecodeString(job.Blob)
	diffTarget := blockchain.TargetToDifficulty(job.Target)
	if err != nil {
		crylog.Error("invalid blob:", job.Blob)
		return
	}

	hash := make([]byte, 32)
	nonce := make([]byte, 4)

	for {
		res := rx.HashUntil(input, uint64(diffTarget), thread, hash, nonce, &stopper)
		if res <= 0 {
			stats.TallyHashes(-res)
			break
		}
		stats.TallyHashes(res)
		crylog.Info("Share found by thread:", thread, "Target:", blockchain.HashDifficulty(hash))
		fnonce := hex.EncodeToString(nonce)
		// submit in a separate thread so we can resume hashing immediately.
		go func(fnonce, jobid string) {
			// If the client isn't alive, then sleep for a bit and hope it wakes up
			// before the share goes stale.
			for {
				if cl.IsAlive() {
					break
				}
				time.Sleep(time.Second)
			}
			resp, err := cl.SubmitWork(fnonce, jobid)
			if err != nil {
				cl.Close()
				crylog.Warn("Submit work client failure:", jobid, err)
				return
			}
			if resp.Error != nil {
				stats.ShareRejected()
				crylog.Warn("Submit work server error:", jobid, resp.Error)
				return
			}
			stats.ShareAccepted(diffTarget)
			swr := resp.Result
			if swr != nil {
				if swr.PoolMargin > 0.0 {
					stats.RefreshPoolStats2(swr)
				} else {
					crylog.Warn("Didn't get pool stats in response:", resp.Result)
					updatePoolStats(true)
				}
			}
		}(fnonce, job.JobID)
	}
}

func OverrideMiningActivityState(mine bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	var newState int
	if mine {
		newState = OVERRIDE_MINE
	} else {
		newState = OVERRIDE_PAUSE
	}
	if miningOverride == newState {
		return
	}
	crylog.Info("Overriding mining state")
	miningOverride = newState
	if plArgs != nil {
		go pokeJobDispatcher(STATE_CHANGE_POKE) // call in own goroutine in case it blocks
	}
}

func RemoveMiningActivityOverride() {
	configMutex.Lock()
	defer configMutex.Unlock()
	if miningOverride == 0 {
		return
	}
	crylog.Info("Removing mining override")
	miningOverride = 0
	if plArgs != nil {
		go pokeJobDispatcher(STATE_CHANGE_POKE) // call in own goroutine in case it blocks
	}
}

func ReportIdleScreenState(isIdle bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if screenIdle == isIdle {
		return
	}
	crylog.Info("Screen idle state changed to:", isIdle)
	screenIdle = isIdle
	if plArgs != nil {
		go pokeJobDispatcher(STATE_CHANGE_POKE) // call in own goroutine in case it blocks
	}
}

func ReportPowerState(battery bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if batteryPower == battery {
		return
	}
	crylog.Info("Battery state changed to:", battery)
	batteryPower = battery
	if plArgs != nil {
		go pokeJobDispatcher(STATE_CHANGE_POKE) // call in own goroutine in case it blocks
	}
}

// configMutex should be locked before calling
func timeExcluded() bool {
	currHr := time.Now().Hour()
	startHr := excludeHourStart
	endHr := excludeHourEnd
	if startHr < endHr {
		return currHr >= startHr && currHr < endHr
	}
	return currHr < startHr && currHr >= endHr
}

func getActivityMessage(activityState int) string {
	switch activityState {
	case MINING_PAUSED_NO_CONNECTION:
		return "PAUSED: no connection."
	case MINING_PAUSED_SCREEN_ACTIVITY:
		return "PAUSED: screen is active."
	case MINING_PAUSED_BATTERY_POWER:
		return "PAUSED: on battery power."
	case MINING_PAUSED_USER_OVERRIDE:
		return "PAUSED: user override."
	case MINING_PAUSED_TIME_EXCLUDED:
		return "PAUSED: within time of day exclusion."
	case MINING_ACTIVE:
		return "ACTIVE"
	case MINING_ACTIVE_USER_OVERRIDE:
		return "ACTIVE: user override."
	}
	crylog.Error("Unknown activity state:", activityState)
	if activityState > 0 {
		return "ACTIVE: unknown reason."
	} else {
		return "PAUSED: unknown reason."
	}
}
