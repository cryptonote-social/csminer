// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

package main

// main() for the osx version of csminer with OSX lock screen state polling.

import (
	"context"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer"
	"os/exec"
	"strings"
	"time"
)

type OSXScreenStater struct {
}

// The OSX implementation of the screen state notification channel is based on polling
// the state every 10 seconds. It would be better to figure out how to get notified
// of state changes when they happen.
func (s OSXScreenStater) GetScreenStateChannel() (chan bool, error) {
	ret := make(chan bool)

	go func() {
		// We assume the screen is active when the miner is started. This may
		// not hold if someone is running the miner from an auto-start script?
		screenActive := true
		for {
			time.Sleep(time.Second * 10)
			screenActiveNow, err := getScreenActiveState()
			if err != nil {
				crylog.Error("getScreenActiveState failed:", err)
				continue
			}
			if screenActiveNow != screenActive {
				screenActive = screenActiveNow
				ret <- !screenActive
			}
		}
	}()
	return ret, nil
}

// getScreenActiveState gets the OSX lockscreen status. Current implementation
// invokes a python script; this should be improved.
func getScreenActiveState() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

func main() {
	csminer.MultiMain(OSXScreenStater{})
}
