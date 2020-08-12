package minerlib

import (
	"github.com/cryptonote-social/csminer/blockchain"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/rx"
	"github.com/cryptonote-social/csminer/stratum/client"

	"bytes"
	"encoding/hex"
	"runtime"
	//	"sync"
	"sync/atomic"
	"time"
)

const (
	//     indicates connection to pool server is lost; miner will continue trying to reconnect.
	MINING_PAUSED_NO_CONNECTION = 2
	//     indicates miner is paused because the screen is active and miner is configured to mine
	//     only when idle.
	MINING_PAUSED_SCREEN_ACTIVITY = 3

	//     indicates miner is paused because the machine is operating on battery power.
	MINING_PAUSED_BATTERY_POWER = 4

	//     indicates miner is paused because of a user-initiated override
	MINING_PAUSED_USER_OVERRIDE = 5

	//     indicates miner is actively mining
	MINING_ACTIVE = 6

	//     indicates miner is actively mining because of user-initiated override
	MINING_ACTIVE_EXTERNAL_OVERRIDE = 7
)

var (
	plArgs *PoolLoginArgs
	cl     client.Client

	screenIdle        int32 // only mine when this is > 0
	batteryPower      int32 // only mine when this is > 0
	manualMinerToggle int32 // whether paused mining has been manually overridden

	// miner client stats
	sharesAccepted                 int64
	sharesRejected                 int64
	poolSideHashes                 int64
	clientSideHashes, recentHashes int64
	startTime                      time.Time // when the miner started up
	lastStatsResetTime             time.Time
	lastStatsUpdateTime            time.Time

	// miner config
	threads int
)

func resetRecentStats() {
	atomic.StoreInt64(&recentHashes, 0)
	lastStatsResetTime = time.Now()
}

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
	r := &PoolLoginResponse{}

	screenIdle = 0
	batteryPower = 0
	manualMinerToggle = 0

	loginName := args.Username
	if args.Wallet != "" {
		loginName = args.Wallet + "." + args.Username
	}
	err, code, message := cl.Connect("cryptonote.social:5555", false /*useTLS*/, args.Agent, loginName, args.Config, args.RigID)
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
	r.Code = 1
	plArgs = args
	return r
}

type StartMinerArgs struct {
	// threads specifies the initial # of threads to mine with. Must be >=1
	Threads int

	// begin/end hours (24 time) of the time during the day where mining should be paused. Set both
	// to 0 if there is no excluded range.
	ExcludeHourStart, ExcludeHourEnd int
}

type StartMinerResponse struct {
	// code == 1: miner started successfully.
	//
	// code == 2: miner started successfully but hugepages could not be enabled, so mining may be
	//            slow. You can suggest to the user that a machine restart might help resolve this.
	//
	// code > 2: miner failed to start due to bad config, see details in message. For example, an
	//           invalid number of threads or invalid hour range may have been specified.
	//
	// code < 0: non-recoverable error, message will provide details. program should exit after
	//           showing message.
	Code    int
	Message string
}

// StartMiner configures the miner and must be called only after a call to PoolLogin was
// successful. You should only call this method once.
func StartMiner(args *StartMinerArgs) *StartMinerResponse {
	r := &StartMinerResponse{}
	hr1 := args.ExcludeHourStart
	hr2 := args.ExcludeHourEnd
	if hr1 > 24 || hr1 < 0 || hr2 > 24 || hr1 < 0 {
		r.Code = 3
		r.Message = "exclude_hour_start and exclude_hour_end must each be between 0 and 24"
		return r
	}
	// Make sure connection was established
	if !cl.IsAlive() {
		r.Code = -1
		r.Message = "StartMiner cannot be called until you first make a successful call to PoolLogin"
		return r
	}
	jobChan, seedHashStr := cl.StartDispatching()
	seed, err := hex.DecodeString(seedHashStr)
	if err != nil {
		// shouldn't happen?
		r.Code = -2
		r.Message = "Could not decode initial RandomX seed hash from pool"
		return r
	}
	code := rx.InitRX(seed, args.Threads, runtime.GOMAXPROCS(0))
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
	startTime = time.Now()
	threads = args.Threads
	go MiningLoop(jobChan, seed)
	return r
}

func reconnectClient(args *PoolLoginArgs) <-chan *client.MultiClientJob {
	loginName := args.Username
	if args.Wallet != "" {
		loginName = args.Wallet + "." + args.Username
	}
	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		err, code, message := cl.Connect("cryptonote.social:5555", false /*useTLS*/, args.Agent, loginName, args.Config, args.RigID)
		if err != nil {
			if code != 0 {
				crylog.Fatal("Pool server did not allow login due to error:", message)
				panic("can't handle pool login error during reconnect")
			}
			crylog.Error("Couldn't connect to pool server:", err)
			time.Sleep(sleepSec)
			sleepSec += time.Second
		}
		jobChan, _ := cl.StartDispatching()
		return jobChan
	}
}

func MiningLoop(jobChan <-chan *client.MultiClientJob, lastSeed []byte) {
	crylog.Info("Mining loop started")

	resetRecentStats()
	var job *client.MultiClientJob
	for {
		select {
		case job = <-jobChan:
			if job == nil {
				crylog.Warn("stratum client died")
				jobChan = reconnectClient(plArgs)
				continue
			}
		}
		crylog.Info("Current job:", job.JobID, "Difficulty:", blockchain.TargetToDifficulty(job.Target))

		// Check if we need to reinitialize rx dataset
		newSeed, err := hex.DecodeString(job.SeedHash)
		if err != nil {
			crylog.Error("invalid seed hash:", job.SeedHash)
			continue
		}
		if bytes.Compare(newSeed, lastSeed) != 0 {
			crylog.Info("New seed:", job.SeedHash)
			rx.InitRX(newSeed, threads, runtime.GOMAXPROCS(0))
			lastSeed = newSeed
			resetRecentStats()
		}
	}

	defer crylog.Info("Mining loop terminated")
}
