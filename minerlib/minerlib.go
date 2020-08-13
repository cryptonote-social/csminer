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
	//	MINING_PAUSED_NO_CONNECTION = -2
	//     indicates connection to pool server is lost; miner will continue trying to reconnect.
	//
	//	MINING_PAUSED_SCREEN_ACTIVITY = -3
	//     indicates miner is paused because the screen is active and miner is configured to mine
	//     only when idle.
	//
	//	MINING_PAUSED_BATTERY_POWER = -4
	//     indicates miner is paused because the machine is operating on battery power.
	//
	//	MINING_PAUSED_USER_OVERRIDE = -5
	//     indicates miner is paused, and is in the "user focred mining pause" state.
	//
	MINING_ACTIVE = 1
	//     indicates miner is actively mining
	//
	//	MINING_ACTIVE_USER_OVERRIDE = 2

)

var (
	// miner config
	configMutex sync.Mutex
	plArgs      *PoolLoginArgs
	threads     int
	loggedIn    bool

	// stratum client
	cl client.Client
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

func PoolLogin(args *PoolLoginArgs) *PoolLoginResponse {
	configMutex.Lock()
	defer configMutex.Unlock()
	loginName := args.Username
	if args.Wallet != "" {
		loginName = args.Wallet + "." + args.Username
	}
	agent := args.Agent
	config := args.Config
	rigid := args.RigID
	loggedIn = false

	r := &PoolLoginResponse{}
	err, code, message := cl.Connect("cryptonote.social:5555", false /*useTLS*/, agent, loginName, config, rigid)
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
	loggedIn = true
	plArgs = args
	r.Code = 1
	go stats.RefreshPoolStats(plArgs.Username)
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
	go func() {
		MiningLoop(awaitLogin())
	}()
	return r
}

func awaitLogin() <-chan *client.MultiClientJob {
	crylog.Info("Awaiting login")
	for {
		configMutex.Lock()
		li := loggedIn
		var uname string
		if plArgs != nil {
			uname = plArgs.Username
		}
		configMutex.Unlock()
		if li {
			crylog.Info("Logged in:", uname)
			return cl.StartDispatching()
		}
		time.Sleep(time.Second)
	}
}

func reconnectClient() <-chan *client.MultiClientJob {
	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		configMutex.Lock()
		if !loggedIn {
			// Client is being reconnected by the user, await until successful.
			configMutex.Unlock()
			return awaitLogin()
		}
		loginName := plArgs.Username
		if plArgs.Wallet != "" {
			loginName = plArgs.Wallet + "." + plArgs.Username
		}
		agent := plArgs.Agent
		config := plArgs.Config
		rigid := plArgs.RigID

		crylog.Info("Reconnecting...")
		err, code, message := cl.Connect("cryptonote.social:5555", false /*useTLS*/, agent, loginName, config, rigid)
		configMutex.Unlock()
		if err == nil {
			return awaitLogin()
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

func MiningLoop(jobChan <-chan *client.MultiClientJob) {
	crylog.Info("Mining loop started")
	lastSeed := []byte{}

	// Synchronization vars
	var wg sync.WaitGroup // used to wait for stopped worker threads to finish
	var stopper uint32    // atomic int used to signal rxlib worker threads to stop mining

	//wasJustMining := false

	for {
		var job *client.MultiClientJob
		select {
		case job = <-jobChan:
			if job == nil {
				crylog.Warn("stratum client died")
				jobChan = reconnectClient()
				atomic.StoreUint32(&stopper, 1)
				wg.Wait()
				stats.ResetRecent()
				continue
			}
		}
		crylog.Info("Current job:", job.JobID, "Difficulty:", blockchain.TargetToDifficulty(job.Target))

		// Stop existing mining, if any, and wait for mining threads to finish.
		atomic.StoreUint32(&stopper, 1)
		wg.Wait()
		printStats(true /*isMining*/)

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
			go goMine(&wg, &stopper, *job, i /*thread*/)
		}
	}

	defer crylog.Info("Mining loop terminated")
}

type GetMiningStateResponse struct {
	stats.Snapshot
	MiningActivity int
	Threads        int
}

func GetMiningState() *GetMiningStateResponse {
	s := stats.GetSnapshot(true /*TODO: fix*/)
	configMutex.Lock()
	defer configMutex.Unlock()
	return &GetMiningStateResponse{
		Snapshot:       *s,
		MiningActivity: MINING_ACTIVE,
		Threads:        threads,
	}
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
	}
	if uname != s.PoolUsername || s.SecondsOld > 120 {
		stats.RefreshPoolStats(uname)
	}
	crylog.Info("=====================================")
}

func goMine(wg *sync.WaitGroup, stopper *uint32, job client.MultiClientJob, thread int) {
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
		res := rx.HashUntil(input, uint64(diffTarget), thread, hash, nonce, stopper)
		if res <= 0 {
			stats.TallyHashes(-res)
			break
		}
		stats.TallyHashes(res)
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
