package minerlib

import (
	"github.com/cryptonote-social/csminer/blockchain"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/minerlib/stats"
	"github.com/cryptonote-social/csminer/rx"
	"github.com/cryptonote-social/csminer/stratum/client"

	"bytes"
	"encoding/hex"
	"runtime"
	"strconv"
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

	OVERRIDE_MINE  = 1
	OVERRIDE_PAUSE = 2
)

var (
	// miner config
	configMutex        sync.Mutex
	plArgs             *PoolLoginArgs
	threads            int
	lastSeed           []byte
	stopMiningLoop     uint32    // for stopping any existing mining loop
	miningLoopDoneChan chan bool // for waiting on an existing mining loop to finish

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
	Code    int
	Message string
}

// See MINING_ACTIVITY const values above for all possibilities. Shorter story: negative value ==
// paused, posiive value == active.
func getMiningActivityState() int {
	configMutex.Lock()
	defer configMutex.Unlock()

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
	configMutex.Lock()
	defer configMutex.Unlock()
	// close out any previous client connection
	if cl.IsAlive() {
		cl.Close()
	}
	// wait for any previous mining loop to terminate
	if miningLoopDoneChan != nil {
		atomic.StoreUint32(&stopMiningLoop, 1)
		<-miningLoopDoneChan
		miningLoopDoneChan = nil
		atomic.StoreUint32(&stopMiningLoop, 0)
	}
	loginName := args.Username
	if args.Wallet != "" {
		loginName = args.Wallet + "." + args.Username
	}
	agent := args.Agent
	config := args.Config
	rigid := args.RigID

	r := &PoolLoginResponse{}
	err, code, message, jc := cl.Connect("cryptonote.social:5555", false /*useTLS*/, agent, loginName, config, rigid)
	if err != nil {
		if code != 0 {
			crylog.Error("Pool server did not allow login due to error:")
			crylog.Error("  ::::::", message, "::::::")
			r.Code = 2
			r.Message = message
			return r
		}
		crylog.Error("Couldn't connect to pool server:", err)
		r.Code = -1
		r.Message = err.Error()
		return r
	} else if code != 0 {
		// We got a warning from the stratum server
		crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::\n")
		if code == client.NO_WALLET_SPECIFIED_WARNING_CODE {
			crylog.Warn("WARNING: your username is not yet associated with any")
			crylog.Warn("   wallet id. You should fix this immediately.")
		} else {
			crylog.Warn("WARNING from pool server")
			crylog.Warn("   Message:", message)
		}
		crylog.Warn("   Code   :", code, "\n")
		crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::\n")
		r.Message = message
	}

	// login successful
	plArgs = args
	r.Code = 1
	go stats.RefreshPoolStats(plArgs.Username)
	miningLoopDoneChan = make(chan bool, 1)
	go MiningLoop(jc)
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
	if hr1 > 24 || hr1 < 0 || hr2 > 24 || hr1 < 0 {
		r.Code = 3
		r.Message = "exclude_hour_start and exclude_hour_end must each be between 0 and 24"
		return r
	}
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
	return r
}

// Returns nil if connection could not be established because of failed call to PoolLogin.
func reconnectClient() <-chan *client.MultiClientJob {
	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		if atomic.LoadUint32(&stopMiningLoop) == 1 {
			// PoolLogin was called since this loop was started, so this mining loop should
			// terminate instead of reconnect.
			return nil
		}
		configMutex.Lock()
		loginName := plArgs.Username
		if plArgs.Wallet != "" {
			loginName = plArgs.Wallet + "." + plArgs.Username
		}
		agent := plArgs.Agent
		config := plArgs.Config
		rigid := plArgs.RigID

		crylog.Info("Attempting to reconnect...")
		err, code, message, jc := cl.Connect("cryptonote.social:5555", false /*useTLS*/, agent, loginName, config, rigid)
		configMutex.Unlock()
		if err == nil {
			return jc
		}
		if code != 0 {
			crylog.Fatal("Pool server did not allow login due to error:", message)
			panic("can't handle pool login error during reconnect")
		}
		crylog.Error("Couldn't connect to pool server:", err)
		crylog.Info("Sleeping for", sleepSec, "seconds before trying again.")
		time.Sleep(sleepSec)
		sleepSec += time.Second
	}
}

// Called by PoolLogin after succesful login.
func MiningLoop(jobChan <-chan *client.MultiClientJob) {
	defer func() { miningLoopDoneChan <- true }()
	crylog.Info("Mining loop started")

	// Set up fresh stats ....
	stopWorkers()
	stats.ResetRecent()

	wasJustMining := false
	var job *client.MultiClientJob
	for {
		select {
		case job = <-jobChan:
			if job == nil {
				crylog.Info("stratum client closed")
				// See if we need to exit this loop or reconnect
				jobChan = reconnectClient()
				if jobChan == nil {
					crylog.Info("Mining loop terminating")
					return
				}
				// Set up fresh stats for new connection
				stopWorkers()
				stats.ResetRecent()
				continue
			}
		case poke := <-pokeChannel:
			pokeRes := handlePoke(true /*wasJustMining*/, poke)
			switch pokeRes {
			case HANDLED:
				continue
			case USE_CACHED:
				if job == nil {
					crylog.Warn("no job to work on")
					continue
				}
			default:
				crylog.Error("mystery poke:", pokeRes)
				continue
			}
		}
		crylog.Info("Current job:", job.JobID, "Difficulty:", blockchain.TargetToDifficulty(job.Target))

		stopWorkers()

		as := getMiningActivityState()
		if as < 0 {
			if wasJustMining {
				printStats(true)
				crylog.Info("Mining is now paused:", as)
				wasJustMining = false
				stats.ResetRecent()
			}
			continue
		}
		if !wasJustMining {
			crylog.Info("Mining is now active:", as)
			wasJustMining = true
			stats.ResetRecent()
		} else {
			printStats(true)
		}

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

		atomic.StoreUint32(&stopper, 0)
		for i := 0; i < threads; i++ {
			wg.Add(1)
			go goMine(*job, i /*thread*/)
		}
	}

	defer crylog.Info("Mining loop terminated")
}

// Stop all active worker threads and wait for them to finish before returning
func stopWorkers() {
	atomic.StoreUint32(&stopper, 1)
	wg.Wait()
}

func handlePoke(wasMining bool, poke int) int {
	crylog.Info("Handling poke:", poke, wasMining)
	if poke == INCREASE_THREADS_POKE {
		stopWorkers()
		configMutex.Lock()
		t := rx.AddThread()
		if t < 0 {
			configMutex.Unlock()
			crylog.Error("Failed to add another thread")
			return USE_CACHED
		}
		threads = t
		configMutex.Unlock()
		crylog.Info("Increased # of threads to:", t)
		stats.ResetRecent()
		return USE_CACHED
	}
	if poke == DECREASE_THREADS_POKE {
		stopWorkers()
		configMutex.Lock()
		t := rx.RemoveThread()
		if t < 0 {
			configMutex.Unlock()
			crylog.Error("Failed to decrease threads")
			return USE_CACHED
		}
		threads = t
		configMutex.Unlock()
		crylog.Info("Decreased # of threads to:", t)
		stats.ResetRecent()
		return USE_CACHED
	}
	if poke == STATE_CHANGE_POKE {
		return USE_CACHED
	}
	return HANDLED
}

type GetMiningStateResponse struct {
	stats.Snapshot
	MiningActivity int
	Threads        int
}

func GetMiningState() *GetMiningStateResponse {
	as := getMiningActivityState()
	var isMining bool
	if as > 0 {
		isMining = true
	}
	s := stats.GetSnapshot(isMining)
	configMutex.Lock()
	defer configMutex.Unlock()
	return &GetMiningStateResponse{
		Snapshot:       *s,
		MiningActivity: as,
		Threads:        threads,
	}
}

func updatePoolStats(isMining bool) {
	s := stats.GetSnapshot(isMining)
	configMutex.Lock()
	uname := plArgs.Username
	configMutex.Unlock()
	if uname != "" && (uname != s.PoolUsername || s.SecondsOld > 5) {
		go stats.RefreshPoolStats(uname)
	}
}

func IncreaseThreads() {
	pokeJobDispatcher(INCREASE_THREADS_POKE)
}

func DecreaseThreads() {
	pokeJobDispatcher(DECREASE_THREADS_POKE)
}

// Poke the job dispatcher. Returns false if the client is not currently alive.
func pokeJobDispatcher(pokeMsg int) {
	crylog.Info("Poking job dispatcher:", pokeMsg)
	pokeChannel <- pokeMsg
}

func printStats(isMining bool) {
	s := stats.GetSnapshot(isMining)
	crylog.Info("=====================================")
	//crylog.Info("Shares    [accepted:rejected]:", s.SharesAccepted, ":", s.SharesRejected)
	//crylog.Info("Hashes          [client:pool]:", s.ClientSideHashes, ":", s.PoolSideHashes)
	if s.RecentHashrate > 0.0 {
		crylog.Info("Hashrate:", strconv.FormatFloat(s.RecentHashrate, 'f', 2, 64))
		//strconv.FormatFloat(s.Hashrate, 'f', 2, 64), ":",
	}
	configMutex.Lock()
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
		updatePoolStats(true)
		crylog.Info("Share found by thread:", thread, "Target:", blockchain.HashDifficulty(hash))
		fnonce := hex.EncodeToString(nonce)
		// If the client is alive, submit the share in a separate thread so we can resume hashing
		// immediately, otherwise wait until it's alive.
		for {
			if cl.IsAlive() {
				break
			}
			time.Sleep(time.Second)
		}
		go func(fnonce, jobid string) {
			resp, err := cl.SubmitWork(fnonce, jobid)
			if err != nil {
				crylog.Warn("Submit work client failure:", jobid, err)
				return
			}
			if len(resp.Error) > 0 {
				stats.ShareRejected()
				crylog.Warn("Submit work server error:", jobid, resp.Error)
				return
			}
			stats.ShareAccepted(diffTarget)
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
	crylog.Info("New mining override state:", newState)
	pokeJobDispatcher(STATE_CHANGE_POKE)
}

func RemoveMiningActivityOverride() {
	configMutex.Lock()
	defer configMutex.Unlock()
	if miningOverride == 0 {
		return
	}
	crylog.Info("Removing mining override")
	miningOverride = 0
}

func ReportIdleScreenState(isIdle bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if screenIdle == isIdle {
		return
	}
	crylog.Info("Screen idle state changed to:", isIdle)
	screenIdle = isIdle
	pokeJobDispatcher(STATE_CHANGE_POKE)
}

func ReportPowerState(battery bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if batteryPower == battery {
		return
	}
	crylog.Info("Battery state changed to:", battery)
	batteryPower = battery
	pokeJobDispatcher(STATE_CHANGE_POKE)
}
