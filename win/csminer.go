// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package main

// main() for the Windows version of csminer with support for Windows locks screen monitoring.

import (
	"github.com/brunoqc/go-windows-session-notifications"
	"github.com/cryptonote-social/csminer"
	"github.com/cryptonote-social/csminer/crylog"
)

type WinScreenStater struct {
}

// We assume the screen is active when the miner is started. This may
// not hold if someone is running the miner from an auto-start script?
// TODO: On startup check if someone is logged in and if not assume screen
// is locked.
func GetScreenStateChannel() (chan bool, error) {
	ret := make(chan bool)

	chanMessages := make(chan session_notifications.Message, 100)
	chanClose := make(chan int)

	go func() {
		for {
			select {
			case m := <-chanMessages:
				switch m.UMsg {
				case session_notifications.WM_WTSSESSION_CHANGE:
					switch m.Param {
					case session_notifications.WTS_SESSION_LOCK:
						crylog.Info("win session locked")
						ret <- true
					case session_notifications.WTS_SESSION_UNLOCK:
						crylog.Info("win session unlocked")
						ret <- false
					default:
					}
				}
				close(m.ChanOk)
			}
		}
		crylog.Error("win screen stater loop exit")
	}()

	session_notifications.Subscribe(chanMessages, chanClose)
	return ret, nil
}

func main() {
	csminer.MultiMain(WinScreenStater{})
}
