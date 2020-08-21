// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package main

// main() for the Windows version of csminer with support for Windows machine state monitoring.

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/brunoqc/go-windows-session-notifications"
	"github.com/cryptonote-social/csminer"
	"github.com/cryptonote-social/csminer/crylog"
	"golang.org/x/sys/windows"
)

type WinMachineStater struct {
	lockedOnStartup bool
}

// We assume the screen is active when the miner is started. This may
// not hold if someone is running the miner from an auto-start script?
func (ss *WinScreenStater) GetMachineStateChannel(saver bool) (chan csminer.MachineState, error) {
	ret := make(chan csminer.MachineState)

	chanClose := make(chan int)
	chanMessages := make(chan session_notifications.Message, 100)

	go func() {
		// TODO: Also monitor for ac vs battery power state
		currentlyLocked := false
		isIdle := false
		batteryPower := false
		for {
			select {
			case m := <-chanMessages:
				switch m.UMsg {
				case session_notifications.WM_WTSSESSION_CHANGE:
					switch m.Param {
					case session_notifications.WTS_SESSION_LOCK:
						crylog.Info("win session locked")
						currentlyLocked = true
						if !isIdle {
							isIdle = true
							ret <- csminer.MachineState(csminer.SCREEN_IDLE)
						}
					case session_notifications.WTS_SESSION_UNLOCK:
						crylog.Info("win session unlocked")
						currentlyLocked = false
						if isIdle {
							isIdle = false
							ret <- csminer.MachineState(csminer.SCREEN_ACTIVE)
						}
					default:
					}
				}
				close(m.ChanOk)
			case <-time.After(10 * time.Second):
				b, err := isBatteryPower()
				if err != nil {
					crylog.Error("failed to get battery power state:", err)
				} else {
					if b != batteryPower {
						if b {
							crylog.Info("Detected battery power")
							batteryPower = true
							ret <- csminer.MachineState(csminer.BATTERY_POWER)
						} else {
							crylog.Info("Detected AC power")
							batteryPower = false
							ret <- csminer.MachineState(csminer.AC_POWER)
						}
					}
				}
				if currentlyLocked {
					continue
				}
				saver, err := isScreenSaverRunning()
				if err != nil {
					crylog.Error("failed to get screensaver state:", err)
					continue
				}
				if saver != isIdle {
					if saver {
						crylog.Info("Detected running screensaver")
						isIdle = true
						ret <- csminer.MachineState(csminer.SCREEN_IDLE)
					} else {
						crylog.Info("No longer detecting active screensaver")
						isIdle = false
						ret <- csminer.MachineState(csminer.SCREEN_ACTIVE)
					}
				}
			}
		}
		crylog.Error("win machine stater loop exit")
	}()

	if saver {
		session_notifications.Subscribe(chanMessages, chanClose)
	}
	return ret, nil
}

func main() {
	ss := WinMachineStater{lockedOnStartup: false}
	csminer.MultiMain(&ss, "csminer "+csminer.VERSION_STRING+" (win)")
}

var libuser32 *windows.LazyDLL
var libkernel32 *windows.LazyDLL

func init() {
	libuser32 = windows.NewLazySystemDLL("user32.dll")
	libkernel32 = windows.NewLazySystemDLL("kernel32.dll")
}

func isScreenSaverRunning() (bool, error) {
	systemParametersInfo := libuser32.NewProc("SystemParametersInfoW")

	var uiAction, uiParam uint32
	uiAction = 0x0072 //SPI_GETSCREENSAVERRUNNING
	var pvParam unsafe.Pointer
	var fWinIni uint32
	var retVal bool
	pvParam = unsafe.Pointer(&retVal)
	res, _, err := syscall.Syscall6(systemParametersInfo.Addr(), 4, uintptr(uiAction), uintptr(uiParam), uintptr(pvParam), uintptr(fWinIni), 0, 0)
	if res == 0 {
		return false, err
	}
	return retVal, nil
}

type systemPowerStatus struct {
	aclineStatus       uint8
	batteryFlag        uint8
	batteryLifePercent uint8
	systemStatusFlag   uint8
	batteryLifeTime    uint32
	batterFullLifeTime uint32
}

func isBatteryPower() (bool, error) {
	getSystemPowerStatus := libkernel32.NewProc("GetSystemPowerStatus")

	var s systemPowerStatus
	res, _, err := syscall.Syscall(getSystemPowerStatus.Addr(), 1, uintptr(unsafe.Pointer(&s)), 0, 0)
	if res == 0 {
		return false, err
	}
	if s.aclineStatus == 0 {
		return true, nil
	}
	return false, nil
}
