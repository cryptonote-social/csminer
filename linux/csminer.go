// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package main

// main() for the Linux version of csminer w/ Gnome screen monitoring support

import (
	"fmt"
	"github.com/cryptonote-social/csminer"
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/godbus/dbus"
)

func main() {
	csminer.MultiMain(GnomeScreenStater{})
}

type GnomeScreenStater struct {
}

func (s GnomeScreenStater) GetScreenStateChannel() (chan csminer.ScreenState, error) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		crylog.Fatal("dbus connection failed")
		return nil, err
	}

	err = bus.AddMatchSignal(
		//		dbus.WithMatchObjectPath("/org/gnome/ScreenSaver"),
		dbus.WithMatchInterface("org.gnome.ScreenSaver"),
		dbus.WithMatchMember("ActiveChanged"),
	)

	dChan := make(chan *dbus.Message, 128)
	bus.Eavesdrop(dChan)

	ret := make(chan csminer.ScreenState)

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
					ret <- csminer.ScreenState(csminer.SCREEN_IDLE)
					continue
				} else if str == "false" {
					crylog.Info("Gnome screensaver turned off")
					ret <- csminer.ScreenState(csminer.SCREEN_ACTIVE)
					continue
				}
			}
			//crylog.Info("ignoring dbus message:", m)
		}
		crylog.Error("dbus listener goroutine exiting")
	}()
	return ret, nil
}
