// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package csminer

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"github.com/cryptonote-social/csminer/blockchain"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/rx"
	"github.com/cryptonote-social/csminer/stratum/client"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	cl          *client.Client
	clMutex     sync.Mutex
	clientAlive bool
	wg          sync.WaitGroup
	stopper     uint32 // atomic int used to signal to rxlib to stop mining

	sharesAccepted                 int64
	sharesRejected                 int64
	poolSideHashes                 int64
	clientSideHashes, recentHashes int64

	startTime           time.Time
	lastStatsResetTime  time.Time
	lastStatsUpdateTime time.Time

	screenIdle        int32 // only mine when this is > 0
	manualMinerToggle int32

	SCREEN_IDLE_POKE client.MultiClientJob
	SCREEN_ON_POKE   client.MultiClientJob
	PRINT_STATS_POKE client.MultiClientJob
	ENTER_HIT_POKE   client.MultiClientJob
)

const (
	HANDLED    = 1
	USE_CACHED = 2
)

type ScreenStater interface {
	// Returns a channel that produces true when state changes from screen off to screen on,
	// and false when it changes from on to off.
	GetScreenStateChannel() (chan bool, error)
}

func Mine(s ScreenStater, threads int, uname, rigid string, saver bool, excludeHrStart int, excludeHrEnd int, startDiff int, useTLS bool) error {
	if useTLS {
		cl = client.NewClient("cryptonote.social:5556")
	} else {
		cl = client.NewClient("cryptonote.social:5555")
	}
	startTime = time.Now()
	lastStatsResetTime = time.Now()
	seed := []byte{}

	screenIdle = 1
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

	startKeyboardScanning()

	for {
		clMutex.Lock()
		clientAlive = false
		clMutex.Unlock()
		sleepSec := 3 * time.Second
		for {
			sd := ""
			if startDiff > 0 {
				sd = "start_diff=" + strconv.Itoa(startDiff)
			}
			err := cl.Connect(uname, sd, rigid, useTLS)
			if err != nil {
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

		crylog.Info("Connected")
		var cachedJob *client.MultiClientJob
		for {
			onoff := getActivityMessage(excludeHrStart, excludeHrEnd, threads)
			crylog.Info("[mining=" + onoff + "]")
			job := <-cl.JobChannel
			if job == nil {
				crylog.Warn("client died")
				time.Sleep(3 * time.Second)
				break
			}
			pokeRes := handlePoke(job, excludeHrStart, excludeHrEnd)
			if pokeRes == HANDLED {
				continue
			}
			if pokeRes == USE_CACHED {
				if cachedJob == nil {
					continue
				}
				job = cachedJob
			} else {
				cachedJob = job
			}
			crylog.Info("Got new job:", job.JobID, "w/ difficulty:", blockchain.TargetToDifficulty(job.Target))

			// Stop existing mining, if any
			atomic.StoreUint32(&stopper, 1)
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

			// Wait until any previous mining is done before starting anew
			wg.Wait()

			// Start mining on new job if screensaver is active
			if miningActive(excludeHrStart, excludeHrEnd) {
				printStats()
				atomic.StoreUint32(&stopper, 0)
				for i := 0; i < threads; i++ {
					wg.Add(1)
					go goMine(&wg, *job, i /*thread*/)
				}
			}
		}
	}
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

func printStats() {
	crylog.Info("Shares    [accepted:rejected]:", sharesAccepted, ":", sharesRejected)
	crylog.Info("Hashes          [client:pool]:", clientSideHashes, ":", poolSideHashes)
	elapsed1 := lastStatsUpdateTime.Sub(startTime).Seconds()
	elapsed2 := lastStatsUpdateTime.Sub(lastStatsResetTime).Seconds()
	if elapsed1 > 0.0 && elapsed2 > 0.0 {
		crylog.Info("Hashes/sec [inception:recent]:",
			strconv.FormatFloat(float64(clientSideHashes)/elapsed1, 'f', 2, 64),
			strconv.FormatFloat(float64(recentHashes)/elapsed2, 'f', 2, 64))
	}
}

func goMine(wg *sync.WaitGroup, job client.MultiClientJob, thread int) {
	defer wg.Done()
	input, err := hex.DecodeString(job.Blob)
	diffTarget := blockchain.TargetToDifficulty(job.Target)
	if err != nil {
		crylog.Error("invalid blob:", job.Blob)
		return
	}

	//crylog.Info("goMine jobid:", job.JobID)

	hash := make([]byte, 32)
	nonce := make([]byte, 4)

	for {
		//crylog.Info("Hashing", diffTarget, thread)
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

func startKeyboardScanning() {
	scanner := bufio.NewScanner(os.Stdin)
	go func() {
		for scanner.Scan() {
			b := scanner.Text()
			if b == "h" || b == "s" {
				kickJobDispatcher(&PRINT_STATS_POKE)
			}
			if b == "q" {
				crylog.Info("quitting due to q key command")
				os.Exit(0)
			}
			if b == "?" {
				crylog.Info("Keyboard commands:")
				crylog.Info("   s: print stats")
				crylog.Info("   q: quit")
				crylog.Info("   <enter>: override a paused miner")
			}
			if len(b) == 0 {
				kickJobDispatcher(&ENTER_HIT_POKE)
			}
			//crylog.Info("Scanned:", string(b), len(b))
		}
		crylog.Error("Scanning terminated")
	}()
}

func monitorScreenSaver(ch chan bool) {
	for state := range ch {
		if state {
			crylog.Info("Screen off")
			kickJobDispatcher(&SCREEN_IDLE_POKE)
		} else {
			crylog.Info("Screen on")
			kickJobDispatcher(&SCREEN_ON_POKE)
		}
	}
}

// kick the job dispatcher with the given "special job". Returns false if the client is not
// currently alive.
func kickJobDispatcher(job *client.MultiClientJob) bool {
	clMutex.Lock()
	defer clMutex.Unlock()
	if !clientAlive {
		return false
	}
	cl.JobChannel <- job
	return true
}

func miningActive(excludeHrStart, excludeHrEnd int) bool {
	saver := atomic.LoadInt32(&screenIdle) > 0
	toggled := atomic.LoadInt32(&manualMinerToggle) > 0
	if !saver || timeExcluded(excludeHrStart, excludeHrEnd) {
		return toggled
	}
	return true
}

func getActivityMessage(excludeHrStart, excludeHrEnd, threads int) string {
	saver := atomic.LoadInt32(&screenIdle) > 0
	toggled := atomic.LoadInt32(&manualMinerToggle) > 0

	onoff := ""
	if timeExcluded(excludeHrStart, excludeHrEnd) {
		if !toggled {
			onoff = "PAUSED due to -exclude hour range. <enter> to mine anyway"
		} else {
			onoff = "ACTIVE due to keyboard override. <enter> to undo override"
		}
	} else if !saver {
		if !toggled {
			onoff = "PAUSED until screen lock. <enter> to mine anyway"
		} else {
			onoff = "ACTIVE due to keyboard override. <enter> to undo override"
		}
	} else {
		onoff = "ACTIVE, threads=" + strconv.Itoa(threads)
	}
	return onoff
}

func handlePoke(job *client.MultiClientJob, excludeHrStart, excludeHrEnd int) int {
	if job == &SCREEN_ON_POKE {
		if miningActive(excludeHrStart, excludeHrEnd) {
			atomic.StoreUint32(&stopper, 1) // halt mining
			wg.Wait()
			printStats()
		} // else mining was already halted
		atomic.StoreInt32(&screenIdle, 0)
		return HANDLED
	}
	if job == &SCREEN_IDLE_POKE {
		if miningActive(excludeHrStart, excludeHrEnd) {
			atomic.StoreInt32(&screenIdle, 1)
			return HANDLED
		} // else mining was already active
		atomic.StoreInt32(&screenIdle, 1)
		resetRecentStats()
		return USE_CACHED
	}
	if job == &ENTER_HIT_POKE {
		if atomic.LoadInt32(&manualMinerToggle) == 0 {
			if miningActive(excludeHrStart, excludeHrEnd) {
				// Ignore enter events if mining is already active due to non-keyboard events
				return HANDLED
			}
			// start up paused miner due to keyboard override
			atomic.StoreInt32(&manualMinerToggle, 1)
			resetRecentStats()
			return USE_CACHED
		}
		if miningActive(excludeHrStart, excludeHrEnd) {
			atomic.StoreUint32(&stopper, 1) // halt mining
			wg.Wait()
			printStats()
		}
		atomic.StoreInt32(&manualMinerToggle, 0)
		return HANDLED
	}
	if job == &PRINT_STATS_POKE {
		atomic.StoreUint32(&stopper, 1) // halt mining to get accurate stats
		wg.Wait()
		printStats()
		return USE_CACHED // resume mining
	}
	return 0
}
