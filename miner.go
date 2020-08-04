// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package csminer

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cryptonote-social/csminer/blockchain"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/rx"
	"github.com/cryptonote-social/csminer/stratum/client"
)

var (
	cl          *client.Client
	clMutex     sync.Mutex
	clientAlive bool // true when the stratum client is connected and healthy

	stopper uint32         // atomic int used to signal rxlib worker threads to stop mining
	wg      sync.WaitGroup // used to wait for stopped worker threads to finish

	pokeChannel chan int // used to send messages to main job loop to take various actions

	// miner client stats
	sharesAccepted                 int64
	sharesRejected                 int64
	poolSideHashes                 int64
	clientSideHashes, recentHashes int64

	startTime           time.Time // when the miner started up
	lastStatsResetTime  time.Time
	lastStatsUpdateTime time.Time

	screenIdle        int32 // only mine when this is > 0
	batteryPower      int32 // only mine when this is > 0
	manualMinerToggle int32 // whether paused mining has been manually overridden

	threads int
)

const (
	HANDLED    = 1
	USE_CACHED = 2

	// Valid screen states
	SCREEN_IDLE   = 0
	SCREEN_ACTIVE = 1
	BATTERY_POWER = 2
	AC_POWER      = 3

	SCREEN_IDLE_POKE      = 0
	SCREEN_ON_POKE        = 1
	BATTERY_POWER_POKE    = 2
	AC_POWER_POKE         = 3
	PRINT_STATS_POKE      = 4
	ENTER_HIT_POKE        = 5
	INCREASE_THREADS_POKE = 6
	DECREASE_THREADS_POKE = 7
)

type ScreenState int

type ScreenStater interface {
	// Returns a channel that produces true when state changes from screen off to screen on,
	// and false when it changes from on to off.
	GetScreenStateChannel() (chan ScreenState, error)
}

func Mine(
	s ScreenStater, t int, uname, rigid string, saver bool,
	excludeHrStart int, excludeHrEnd int, startDiff int,
	useTLS bool, config string, agent string) error {
	threads = t
	if useTLS {
		cl = client.NewClient("cryptonote.social:5556", agent)
	} else {
		cl = client.NewClient("cryptonote.social:5555", agent)
	}
	startTime = time.Now()
	lastStatsResetTime = time.Now()
	seed := []byte{}

	screenIdle = 1
	batteryPower = 0

	manualMinerToggle = 0
	if saver {
		ch, err := s.GetScreenStateChannel()
		if err != nil {
			crylog.Error("failed to get screen state monitor, screen state will be ignored")
		} else {
			// We assume the screen is active when the miner is started. This may
			// not hold if someone is running the miner from an auto-start script?
			screenIdle = 0
			go monitorScreenSaver(ch)
		}
	}

	printKeyboardCommands()
	startKeyboardScanning(uname)

	wasJustMining := false
	pokeChannel = make(chan int, 5) // use small amount of buffering for when internet may be bad

outer:
	for {
		jobChannel := connectClient(cl, uname, rigid, startDiff, config, useTLS)
		resetRecentStats()
		var job *client.MultiClientJob
		for {
			onoff := getActivityMessage(excludeHrStart, excludeHrEnd, threads)
			crylog.Info("[mining=" + onoff + "]")
			select {
			case poke := <-pokeChannel:
				pokeRes := handlePoke(wasJustMining, poke, excludeHrStart, excludeHrEnd)
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
			case job = <-jobChannel:
				if job == nil {
					crylog.Warn("stratum client died, reconnecting")
					continue outer
				}
			}
			crylog.Info("Current job:", job.JobID, "Difficulty:", blockchain.TargetToDifficulty(job.Target))

			// Stop existing mining, if any, and wait for mining threads to finish.
			atomic.StoreUint32(&stopper, 1)
			wg.Wait()

			// Check if we need to reinitialize rx dataset
			newSeed, err := hex.DecodeString(job.SeedHash)
			if err != nil {
				crylog.Error("invalid seed hash:", job.SeedHash)
				continue
			}
			if bytes.Compare(newSeed, seed) != 0 {
				crylog.Info("New seed:", job.SeedHash)
				rx.InitRX(newSeed, threads, runtime.GOMAXPROCS(0))
				seed = newSeed
				resetRecentStats()
			}

			// Start mining on new job if mining is active
			if miningActive(excludeHrStart, excludeHrEnd) {
				if !wasJustMining {
					// Reset recent stats whenever going from not mining to mining,
					// and don't print stats because they could be inaccurate
					resetRecentStats()
					wasJustMining = true
				} else {
					printStats(true)
				}
				atomic.StoreUint32(&stopper, 0)
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go goMine(&wg, *job, i /*thread*/)
				}
			} else if wasJustMining {
				// Print stats whenever we go from mining to not mining
				printStats(false)
				wasJustMining = false
			}
		}
	}
}

// connectClient will try to establish the connection to the stratum server and won't return until
// successful. It will also start the job dispatching loop once connected. clientAlive will be true
// upon return.
func connectClient(cl *client.Client, uname, rigid string, startDiff int, config string, useTLS bool) chan *client.MultiClientJob {
	clMutex.Lock()
	clientAlive = false
	clMutex.Unlock()
	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		if startDiff > 0 {
			if len(config) > 0 {
				config += ";"
			}
			config += "start_diff=" + strconv.Itoa(startDiff)
		}
		clMutex.Lock()
		err := cl.Connect(uname, config, rigid, useTLS)
		clMutex.Unlock()
		if err != nil {
			errString := err.Error()
			if strings.Index(errString, client.STRATUM_SERVER_ERROR) == 0 {
				crylog.Error("Pool server returned error:", errString[len(client.STRATUM_SERVER_ERROR):])
				os.Exit(1)
			}
			crylog.Warn("Client failed to connect:", err)
			time.Sleep(sleepSec)
			sleepSec += time.Second
			continue
		}
		break
	}
	clMutex.Lock()
	clientAlive = true
	clMutex.Unlock()

	go func() {
		err := cl.DispatchJobs()
		if err != nil {
			crylog.Error("Job dispatcher exitted with error:", err)
			os.Exit(1)
		}
		clMutex.Lock()
		if clientAlive {
			clientAlive = false
			cl.Close()
		}
		clMutex.Unlock()
	}()

	crylog.Info("Connected")
	return cl.JobChannel
}

func timeExcluded(startHr, endHr int) bool {
	currHr := time.Now().Hour()
	if startHr < endHr {
		return currHr >= startHr && currHr < endHr
	}
	return currHr < startHr && currHr >= endHr
}

func resetRecentStats() {
	atomic.StoreInt64(&recentHashes, 0)
	lastStatsResetTime = time.Now()
}

func printStats(isMining bool) {
	crylog.Info("=====================================")
	crylog.Info("Shares    [accepted:rejected]:", sharesAccepted, ":", sharesRejected)
	crylog.Info("Hashes          [client:pool]:", clientSideHashes, ":", poolSideHashes)
	var elapsed1 float64
	if isMining {
		// if we're actively mining then hash count is only accurate
		// as of the last update time
		elapsed1 = lastStatsUpdateTime.Sub(startTime).Seconds()
	} else {
		elapsed1 = time.Now().Sub(startTime).Seconds()
	}
	elapsed2 := lastStatsUpdateTime.Sub(lastStatsResetTime).Seconds()
	if elapsed1 > 0.0 && elapsed2 > 0.0 {
		crylog.Info("Hashes/sec [inception:recent]:",
			strconv.FormatFloat(float64(clientSideHashes)/elapsed1, 'f', 2, 64), ":",
			strconv.FormatFloat(float64(recentHashes)/elapsed2, 'f', 2, 64))
	}
	crylog.Info("=====================================")
}

func goMine(wg *sync.WaitGroup, job client.MultiClientJob, thread int) {
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
		lastStatsUpdateTime = time.Now()
		if res <= 0 {
			atomic.AddInt64(&clientSideHashes, -res)
			atomic.AddInt64(&recentHashes, -res)
			break
		}
		atomic.AddInt64(&clientSideHashes, res)
		atomic.AddInt64(&recentHashes, res)
		crylog.Info("Share found:", blockchain.HashDifficulty(hash), thread)
		fnonce := hex.EncodeToString(nonce)

		// If the client is alive, submit the share in a separate thread so we can resume hashing
		// immediately, otherwise wait until it's alive.
		for {
			var alive bool
			clMutex.Lock()
			alive = clientAlive
			clMutex.Unlock()
			if alive {
				break
			}
			//crylog.Warn("client ded")
			time.Sleep(time.Second)
		}

		go func(fnonce, jobid string) {
			clMutex.Lock()
			defer clMutex.Unlock()
			if !clientAlive {
				crylog.Warn("client died, abandoning work")
				return
			}
			resp, err := cl.SubmitWork(fnonce, jobid)
			if err != nil {
				crylog.Warn("Submit work client failure:", jobid, err)
				clientAlive = false
				cl.Close()
				return
			}
			if len(resp.Error) > 0 {
				atomic.AddInt64(&sharesRejected, 1)
				crylog.Warn("Submit work server error:", jobid, resp.Error)
				return
			}
			atomic.AddInt64(&sharesAccepted, 1)
			atomic.AddInt64(&poolSideHashes, diffTarget)
		}(fnonce, job.JobID)
	}
}

func printKeyboardCommands() {
	crylog.Info("Keyboard commands:")
	crylog.Info("   s: print miner stats")
	crylog.Info("   p: print pool-side user stats")
	crylog.Info("   i/d: increase/decrease number of threads by 1")
	crylog.Info("   q: quit")
	crylog.Info("   <enter>: override a paused miner")
}

func startKeyboardScanning(uname string) {
	scanner := bufio.NewScanner(os.Stdin)
	go func() {
		for scanner.Scan() {
			b := scanner.Text()
			switch b {
			case "i":
				pokeJobDispatcher(INCREASE_THREADS_POKE)
			case "d":
				pokeJobDispatcher(DECREASE_THREADS_POKE)
			case "h", "s":
				pokeSuccess := pokeJobDispatcher(PRINT_STATS_POKE)
				if !pokeSuccess {
					// stratum client is probably dead due to bad internet connection, but it should
					// still be safe to print stats.
					printStats(false)
				}
			case "q", "quit", "exit":
				crylog.Info("quitting due to keyboard command")
				os.Exit(0)
			case "?", "help":
				printKeyboardCommands()
			case "p":
				err := printPoolSideStats(uname)
				if err != nil {
					crylog.Error("Failed to get pool side user stats:", err)
				}
			}
			if len(b) == 0 {
				// Ignore enter-hit mining override if on battery power
				if atomic.LoadInt32(&batteryPower) > 0 {
					crylog.Warn("on battery power, keyboard overrides ignored")
					continue
				}
				pokeSuccess := pokeJobDispatcher(ENTER_HIT_POKE)
				if pokeSuccess {
					if atomic.LoadInt32(&manualMinerToggle) == 0 {
						atomic.StoreInt32(&manualMinerToggle, 1)
					} else {
						atomic.StoreInt32(&manualMinerToggle, 0)
					}
				}
			}
		}
		crylog.Error("Scanning terminated")
	}()
}

func printPoolSideStats(uname string) error {
	c := &http.Client{
		Timeout: 15 * time.Second,
	}
	uri := "https://cryptonote.social/json/WorkerStats"
	sbody := "{\"Coin\": \"xmr\", \"Worker\": \"" + uname + "\"}\n"
	body := strings.NewReader(sbody)
	resp, err := c.Post(uri, "", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	s := &struct {
		Code             int
		CycleProgress    float64
		Hashrate1        int64
		Hashrate24       int64
		LifetimeHashes   int64
		LifetimeBestHash int64
		Donate           float64
		AmountPaid       float64
		AmountOwed       float64
	}{}
	err = json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	// Now get pool stats
	uri = "https://cryptonote.social/json/PoolStats"
	sbody = "{\"Coin\": \"xmr\"}\n"
	body = strings.NewReader(sbody)
	resp2, err := http.DefaultClient.Post(uri, "", body)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	b, err = ioutil.ReadAll(resp2.Body)
	if err != nil {
		return err
	}
	ps := &struct {
		Code               int
		NextBlockReward    float64
		Margin             float64
		PPROPProgress      float64
		PPROPHashrate      int64
		NetworkDifficulty  int64
		SmoothedDifficulty int64 // Network difficulty averaged over the past hour
	}{}
	err = json.Unmarshal(b, &ps)
	if err != nil {
		return err
	}

	s.CycleProgress /= (1.0 + ps.Margin)

	diff := float64(ps.SmoothedDifficulty)
	if diff == 0.0 {
		diff = float64(ps.NetworkDifficulty)
	}
	hr := float64(ps.PPROPHashrate)
	var ttreward string
	if hr > 0.0 {
		ttr := (diff*(1.0+ps.Margin) - (ps.PPROPProgress * diff)) / hr / 3600.0 / 24.0
		if ttr > 0.0 {
			if ttr < 1.0 {
				ttr *= 24.0
				if ttr < 1.0 {
					ttr *= 60.0
					ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " min"
				} else {
					ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " hrs"
				}
			} else {
				ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " days"
			}
		} else if ttr < 0.0 {
			ttreward = "overdue"
		}
	}

	crylog.Info("==========================================")
	crylog.Info("User            :", uname)
	crylog.Info("Progress        :", strconv.FormatFloat(s.CycleProgress*100.0, 'f', 5, 64)+"%")
	crylog.Info("1 Hr Hashrate   :", s.Hashrate1)
	crylog.Info("24 Hr Hashrate  :", s.Hashrate24)
	crylog.Info("Lifetime Hashes :", prettyInt(s.LifetimeHashes))
	crylog.Info("Paid            :", strconv.FormatFloat(s.AmountPaid, 'f', 12, 64), "$XMR")
	if s.AmountOwed > 0.0 {
		crylog.Info("Amount Owed     :", strconv.FormatFloat(s.AmountOwed, 'f', 12, 64), "$XMR")
	}
	/*crylog.Info("PPROP Progress         :", strconv.FormatFloat(ps.PPROPProgress*100.0, 'f', 5, 64)+"%")*/
	crylog.Info("")
	crylog.Info("Estimated stats :")
	if len(ttreward) > 0 {
		crylog.Info("  Time to next reward:", ttreward)
	}
	if ps.NextBlockReward > 0.0 && s.CycleProgress > 0.0 {
		crylog.Info("  Reward accumulated :", strconv.FormatFloat(ps.NextBlockReward*s.CycleProgress, 'f', 12, 64), "$XMR")
	}
	crylog.Info("==========================================")

	return nil
}

func prettyInt(i int64) string {
	s := strconv.Itoa(int(i))
	out := []byte{}
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if count == 3 {
			out = append(out, ',')
			count = 0
		}
		out = append(out, s[i])
		count++
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func monitorScreenSaver(ch chan ScreenState) {
	for state := range ch {
		switch state {
		case SCREEN_IDLE:
			crylog.Info("Screen idle")
			atomic.StoreInt32(&screenIdle, 1)
			pokeJobDispatcher(SCREEN_IDLE_POKE)
		case SCREEN_ACTIVE:
			crylog.Info("Screen active")
			atomic.StoreInt32(&screenIdle, 0)
			pokeJobDispatcher(SCREEN_ON_POKE)
		case BATTERY_POWER:
			crylog.Info("Battery power")
			atomic.StoreInt32(&batteryPower, 1)
			pokeJobDispatcher(BATTERY_POWER_POKE)
		case AC_POWER:
			crylog.Info("AC power")
			atomic.StoreInt32(&batteryPower, 0)
			pokeJobDispatcher(AC_POWER_POKE)
		}
	}
}

// Poke the job dispatcher. Returns false if the client is not currently alive.
func pokeJobDispatcher(pokeMsg int) bool {
	clMutex.Lock()
	alive := clientAlive
	clMutex.Unlock()
	if !alive {
		// jobs will block, so just ignore
		crylog.Warn("Stratum client is not alive. Ignoring poke:", pokeMsg)
		return false
	}
	pokeChannel <- pokeMsg
	return true
}

func miningActive(excludeHrStart, excludeHrEnd int) bool {
	if atomic.LoadInt32(&batteryPower) > 0 {
		return false
	}
	if atomic.LoadInt32(&manualMinerToggle) > 0 {
		// keyboard override to always mine no matter what
		return true
	}
	if atomic.LoadInt32(&screenIdle) == 0 {
		// don't mine if screen is active
		return false
	}
	if timeExcluded(excludeHrStart, excludeHrEnd) {
		// don't mine if we're in the excluded time range
		return false
	}
	return true
}

func getActivityMessage(excludeHrStart, excludeHrEnd, threads int) string {
	battery := atomic.LoadInt32(&batteryPower) > 0
	if battery {
		return "PAUSED due to running on battery power"
	}

	saver := atomic.LoadInt32(&screenIdle) > 0
	toggled := atomic.LoadInt32(&manualMinerToggle) > 0

	onoff := ""

	if timeExcluded(excludeHrStart, excludeHrEnd) {
		if !toggled {
			onoff = "PAUSED due to -exclude hour range. <enter> to mine anyway"
		} else {
			onoff = "ACTIVE (threads=" + strconv.Itoa(threads) + ") due to keyboard override. <enter> to undo override"
		}
	} else if !saver {
		if !toggled {
			onoff = "PAUSED until screen lock. <enter> to mine anyway"
		} else {
			onoff = "ACTIVE (threads=" + strconv.Itoa(threads) + ") due to keyboard override. <enter> to undo override"
		}
	} else {
		onoff = "ACTIVE (threads=" + strconv.Itoa(threads) + ")"
	}
	return onoff
}

func handlePoke(wasMining bool, poke int, excludeHrStart, excludeHrEnd int) int {
	if poke == PRINT_STATS_POKE {
		if !wasMining {
			printStats(wasMining)
			return HANDLED
		}
		// If we are actively mining we'll want to accumulate stats from workers before printing
		// stats for accuracy, so just trigger a fall-through and main loop will sync+dump stats.
		return USE_CACHED
	}
	if poke == INCREASE_THREADS_POKE {
		atomic.StoreUint32(&stopper, 1)
		wg.Wait()
		t := rx.AddThread()
		if t < 0 {
			crylog.Error("Failed to add another thread")
			return USE_CACHED
		}
		threads = t
		crylog.Info("Increased # of threads to:", threads)
		resetRecentStats()
		return USE_CACHED
	}
	if poke == DECREASE_THREADS_POKE {
		atomic.StoreUint32(&stopper, 1)
		wg.Wait()
		t := rx.RemoveThread()
		if t < 0 {
			crylog.Error("Failed to decrease threads")
			return USE_CACHED
		}
		threads = t
		crylog.Info("Decreased # of threads to:", threads)
		resetRecentStats()
		return USE_CACHED
	}
	isMiningNow := miningActive(excludeHrStart, excludeHrEnd)
	if wasMining != isMiningNow {
		// mining state was toggled so fall through using last received job which will
		// appropriately halt or restart any mining threads and/or print stats.
		return USE_CACHED
	}
	return HANDLED
}
