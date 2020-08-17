// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package csminer

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/minerlib"
	"github.com/cryptonote-social/csminer/stratum/client"
)

var ()

const (
	// Valid screen states
	SCREEN_IDLE   = 0
	SCREEN_ACTIVE = 1
	BATTERY_POWER = 2
	AC_POWER      = 3
)

type ScreenState int

type ScreenStater interface {
	// Returns a channel that produces true when state changes from screen off to screen on,
	// and false when it changes from on to off.
	GetScreenStateChannel() (chan ScreenState, error)
}

type MinerConfig struct {
	ScreenStater                 ScreenStater
	Threads                      int
	Username, RigID              string
	Wallet                       string
	Agent                        string
	Saver                        bool
	ExcludeHrStart, ExcludeHrEnd int
	StartDiff                    int // deprecated, use AdvancedConfig instead
	UseTLS                       bool
	AdvancedConfig               string
}

func Mine(c *MinerConfig) error {
	imResp := minerlib.InitMiner(&minerlib.InitMinerArgs{
		Threads:          1,
		ExcludeHourStart: c.ExcludeHrStart,
		ExcludeHourEnd:   c.ExcludeHrEnd,
	})
	if imResp.Code > 2 {
		crylog.Error("Bad configuration:", imResp.Message)
		return errors.New("InitMiner failed: " + imResp.Message)
	}
	if imResp.Code == 2 {
		crylog.Warn("")
		crylog.Warn("WARNING: Could not allocate hugepages. Mining might be slow. A reboot might help.")
		crylog.Warn("")
	}

	sleepSec := 3 * time.Second // time to sleep if connection attempt fails
	for {
		plResp := minerlib.PoolLogin(&minerlib.PoolLoginArgs{
			Username: c.Username,
			RigID:    c.RigID,
			Wallet:   c.Wallet,
			Agent:    c.Agent,
			Config:   c.AdvancedConfig,
			UseTLS:   c.UseTLS,
		})
		if plResp.Code < 0 {
			crylog.Error("Pool server not responding:", plResp.Message)
			crylog.Info("Sleeping for", sleepSec, "seconds before trying again.")
			time.Sleep(sleepSec)
			sleepSec += time.Second
			continue
		}
		if plResp.Code == 1 {
			if len(plResp.Message) > 0 {
				crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::\n")
				if plResp.MessageID == client.NO_WALLET_SPECIFIED_WARNING_CODE {
					crylog.Warn("WARNING: your username is not yet associated with any")
					crylog.Warn("   wallet id. You should fix this immediately.")
				} else {
					crylog.Warn("WARNING from pool server")
					crylog.Warn("   Message:", plResp.Message)
				}
				crylog.Warn("   Code   :", plResp.MessageID, "\n")
				crylog.Warn(":::::::::::::::::::::::::::::::::::::::::::::::::::::::::")
			}
			break
		}
		crylog.Error("Pool refused login:", plResp.Message)
		return errors.New("pool refused login")
	}

	if c.Saver {
		ch, err := c.ScreenStater.GetScreenStateChannel()
		if err != nil {
			crylog.Error("failed to get screen state monitor, screen state will be ignored")
		} else {
			// We assume the screen is active when the miner is started. This may
			// not hold if someone is running the miner from an auto-start script?
			go monitorScreenSaver(ch)
		}
	} else {
		minerlib.ReportIdleScreenState(true)
	}

	printKeyboardCommands()

	scanner := bufio.NewScanner(os.Stdin)
	var manualMinerActivate bool
	for scanner.Scan() {
		b := scanner.Text()
		switch b {
		case "i":
			crylog.Info("Increasing thread count.")
			minerlib.IncreaseThreads()
		case "d":
			crylog.Info("Decreasing thread count.")
			minerlib.DecreaseThreads()
		case "h", "s", "p":
			printStats()
		case "q", "quit", "exit":
			crylog.Info("quitting due to keyboard command")
			return nil
		case "?", "help":
			printKeyboardCommands()
		}
		if len(b) == 0 {
			if !manualMinerActivate {
				manualMinerActivate = true
				crylog.Info("Overriding mining state to ACTIVE")
				minerlib.OverrideMiningActivityState(true)
			} else {
				crylog.Info("Removing mining override")
				manualMinerActivate = false
				minerlib.RemoveMiningActivityOverride()
			}
		}
	}
	crylog.Error("Scanning terminated")
	return errors.New("didn't expect keyboard scanning to terminate")
}

func printStats() {
	s := minerlib.GetMiningState()
	msg := getActivityMessage(s.MiningActivity)
	crylog.Info("")
	crylog.Info("===============================================================================")
	if s.RecentHashrate < 0 {
		crylog.Info("Current Hashrate             : --calculating--")
	} else {
		crylog.Info("Current Hashrate              :", strconv.FormatFloat(s.RecentHashrate, 'f', 2, 64))
	}
	crylog.Info("Hashrate since inception     :", strconv.FormatFloat(s.Hashrate, 'f', 2, 64))
	crylog.Info("Threads                      :", s.Threads)
	crylog.Info("===============================================================================")
	crylog.Info("Shares    [accepted:rejected]:", s.SharesAccepted, ":", s.SharesRejected)
	crylog.Info("Hashes          [client:pool]:", s.ClientSideHashes, ":", s.PoolSideHashes)
	crylog.Info("===============================================================================")
	if s.SecondsOld >= 0.0 {
		crylog.Info("Pool username                :", s.PoolUsername)
		crylog.Info("Last pool stats refresh      :", s.SecondsOld, "seconds ago")
		crylog.Info("  Lifetime hashes            :", prettyInt(s.LifetimeHashes))
		crylog.Info("  Paid                       :", strconv.FormatFloat(s.Paid, 'f', 12, 64), "$XMR")
		if s.Owed > 0.0 {
			crylog.Info("  Owed                       :", strconv.FormatFloat(s.Owed, 'f', 12, 64), "$XMR")
		}
		crylog.Info("  Time to next reward (est.) :", s.TimeToReward)
		crylog.Info("    Accumulated (est.)       :", strconv.FormatFloat(s.Accumulated, 'f', 12, 64), "$XMR")
		crylog.Info("===============================================================================")
	}
	crylog.Info("Mining", msg)
	crylog.Info("===============================================================================")
	crylog.Info("")
}

func printKeyboardCommands() {
	crylog.Info("Keyboard commands:")
	crylog.Info("   s: print miner stats")
	//crylog.Info("   p: print pool-side user stats")
	crylog.Info("   i/d: increase/decrease number of threads by 1")
	crylog.Info("   q: quit")
	crylog.Info("   <enter>: override a paused miner")
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
			minerlib.ReportIdleScreenState(true)
		case SCREEN_ACTIVE:
			minerlib.ReportIdleScreenState(false)
		case BATTERY_POWER:
			minerlib.ReportPowerState(true)
		case AC_POWER:
			minerlib.ReportPowerState(false)
		}
	}
}

func getActivityMessage(activityState int) string {
	switch activityState {
	case minerlib.MINING_PAUSED_NO_CONNECTION:
		return "PAUSED: no connection."
	case minerlib.MINING_PAUSED_SCREEN_ACTIVITY:
		return "PAUSED: screen is active. Hit <enter> to override."
	case minerlib.MINING_PAUSED_BATTERY_POWER:
		return "PAUSED: on battery power. Hit <enter> to override."
	case minerlib.MINING_PAUSED_USER_OVERRIDE:
		return "PAUSED: keyboard override active. Hit <enter> to remove override."
	case minerlib.MINING_PAUSED_TIME_EXCLUDED:
		return "PAUSED: within time of day exclusion. Hit <enter> to override."
	case minerlib.MINING_ACTIVE:
		return "ACTIVE"
	case minerlib.MINING_ACTIVE_USER_OVERRIDE:
		return "ACTIVE: keyboard override active. Hit <enter> to remove override."
	}
	crylog.Fatal("Unknown activity state:", activityState)
	if activityState > 0 {
		return "ACTIVE: unknown reason"
	} else {
		return "PAUSED: unknown reason"
	}
}
