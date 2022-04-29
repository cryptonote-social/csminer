// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package main

// main() for the Linux version of csminer w/ Gnome screen monitoring support

import (
	"fmt"
	"github.com/cryptonote-social/csminer"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/godbus/dbus/v5"
)

func main() {
	csminer.MultiMain(GnomeMachineStater{}, "csminer "+csminer.VERSION_STRING+" (linux)")
}

type GnomeMachineStater struct {
}

func (s GnomeMachineStater) GetMachineStateChannel(saver bool) (chan csminer.MachineState, error) {
	ret := make(chan csminer.MachineState)
	if !saver {
		return ret, nil // return channel on which we never send updates
	}
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		crylog.Error("dbus connection failed")
		return nil, err
	}

	err = bus.AddMatchSignal(
		//		dbus.WithMatchObjectPath("/org/gnome/ScreenSaver"),
		dbus.WithMatchInterface("org.gnome.ScreenSaver"),
		dbus.WithMatchMember("ActiveChanged"),
	)

	dChan := make(chan *dbus.Message, 128)
	bus.Eavesdrop(dChan)

	go func() {
		defer bus.Close()
		for m := range dChan {
			if m == nil {
				crylog.Warn("got nil message")
				continue
			}
			if len(m.Body) > 0 {
				str := fmt.Sprintf("%v", m.Body[0])
				if str == "true" {
					crylog.Info("Gnome screensaver turned on")
					ret <- csminer.MachineState(csminer.SCREEN_IDLE)
					continue
				} else if str == "false" {
					crylog.Info("Gnome screensaver turned off")
					ret <- csminer.MachineState(csminer.SCREEN_ACTIVE)
					continue
				}
			}
			//crylog.Info("ignoring dbus message:", m)
		}
		crylog.Error("dbus listener goroutine exiting")
	}()
	return ret, nil
}
