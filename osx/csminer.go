// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

package main

// main() for the osx version of csminer with OSX lock screen & battery state polling.

import (
	"context"
	"github.com/cryptonote-social/csminer"
	"github.com/cryptonote-social/csminer/crylog"
	"os/exec"
	"strings"
	"time"
)

type OSXMachineStater struct {
}

// The OSX implementation of the screen & batter state notification channel is based on polling the
// state every 10 seconds. It would be better to figure out how to get notified of state changes
// when they happen.
func (s OSXMachineStater) GetMachineStateChannel(saver bool) (chan csminer.MachineState, error) {
	ret := make(chan csminer.MachineState)

	go func() {
		screenActive := true
		batteryPower := false
		for {
			time.Sleep(time.Second * 5)
			if saver {
				screenActiveNow, err := getScreenActiveState()
				if err != nil {
					crylog.Error("getScreenActiveState failed:", err)
					continue
				}
				if screenActiveNow != screenActive {
					screenActive = screenActiveNow
					if screenActive {
						ret <- csminer.MachineState(csminer.SCREEN_ACTIVE)
					} else {
						ret <- csminer.MachineState(csminer.SCREEN_IDLE)
					}
				}
			}
			time.Sleep(time.Second * 5)
			batteryPowerNow, err := getBatteryPowerState()
			if err != nil {
				crylog.Error("getBatteryPowerState failed:", err)
				continue
			}
			if batteryPower != batteryPowerNow {
				batteryPower = batteryPowerNow
				if batteryPower {
					ret <- csminer.MachineState(csminer.BATTERY_POWER)
				} else {
					ret <- csminer.MachineState(csminer.AC_POWER)
				}
			}
		}
	}()
	return ret, nil
}

// getScreenActiveState gets the OSX lockscreen status. Current implementation
// invokes a python script; this should be improved.
func getScreenActiveState() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"python",
		"-c",
		"import sys,Quartz; d=Quartz.CGSessionCopyCurrentDictionary(); print d",
	)

	b, err := cmd.CombinedOutput()
	if err != nil {
		crylog.Error("Error in cmd.CombinedOutput:", err)
		return false, err
	}

	if strings.Contains(string(b), "CGSSessionScreenIsLocked = 1") {
		return false, nil
	}
	return true, nil
}

// getBatteryPowerState returns true if the machine is running on battery power.
// Current implementation invokes "pmset -g ps"
func getBatteryPowerState() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"pmset",
		"-g",
		"ps",
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		crylog.Error("Error in cmd.CombinedOutput:", err)
		return false, err
	}
	if strings.Contains(string(b), "Battery Power") {
		return true, nil
	}
	return false, nil
}

func main() {
	csminer.MultiMain(OSXMachineStater{}, "csminer "+csminer.VERSION_STRING+" (osx)")
}
