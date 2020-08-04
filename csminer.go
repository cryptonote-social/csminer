// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package csminer

import (
	"flag"
	"fmt"
	"github.com/cryptonote-social/csminer/crylog"
	"strconv"
	"strings"
)

const (
	APPLICATION_NAME = "cryptonote.social Monero miner"
	VERSION_STRING   = "0.1.1"
	STATS_WEBPAGE    = "https://cryptonote.social/xmr"
	DONATE_USERNAME  = "donate-getmonero-org"

	INVALID_EXCLUDE_FORMAT_MESSAGE = "invalid format for exclude specified. Specify XX-YY, e.g. 11-16 for 11:00am to 4:00pm."
)

var (
	saver   = flag.Bool("saver", true, "run only when screen is locked")
	t       = flag.Int("threads", 1, "number of threads")
	uname   = flag.String("user", DONATE_USERNAME, "your pool username from https://cryptonote.social/xmr")
	rigid   = flag.String("rigid", "csminer", "your rig id")
	tls     = flag.Bool("tls", false, "whether to use TLS when connecting to the pool")
	exclude = flag.String("exclude", "", "pause mining during these hours, e.g. -exclude=11-16 will pause mining between 11am and 4pm")
	config  = flag.String("config", "", "advanced pool configuration options, e.g. start_diff=1000;donate=1.0")

	// Deprecated:
	startDiff = flag.Int("start_diff", 0, "a starting difficulty value for the pool")
)

func MultiMain(s ScreenStater, agent string) {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "==== %s %s ====\n", APPLICATION_NAME, VERSION_STRING)
		fmt.Fprint(flag.CommandLine.Output(),
			`Usage of ./csminer
  -user <string>
    	your pool username from https://cryptonote.social/xmr (default "donate-getmonero-org")
  -saver=<bool>
    	mine only when screen is locked (default true)
  -exclude <string>
        pause mining during the specified hours. Format is XX-YY where XX and YY are hours of
        the day designated in 24 hour time. For example, -exclude=11-16 will pause mining betwen
        11:00am and 4:00pm. This can be used, for example, to pause mining during times of high
        machine usage or high electricity rates.
  -threads <int>
    	number of threads (default 1)
  -rigid <string>
    	your rig id (default "csminer")
  -tls <bool>
       whether to use TLS when connecting to the pool (default false)
  -config <string>
        advanced pool config option string, for specifying starting diff, donation percentage,
        email address for notifications, and more. See "advanced configuration options" under Get
        Started on the pool site for details.
`)
		fmt.Fprintf(flag.CommandLine.Output(), "\nMonitor your miner progress at: %s\n", STATS_WEBPAGE)
		fmt.Fprint(flag.CommandLine.Output(), "Send feedback to: cryptonote.social@gmail.com\n")
	}
	flag.Parse()

	var hr1, hr2 int
	hr1 = -1
	var err error
	if len(*exclude) > 0 {
		hrs := strings.Split(*exclude, "-")
		if len(hrs) != 2 {
			crylog.Error(INVALID_EXCLUDE_FORMAT_MESSAGE)
			return
		}
		hr1, err = strconv.Atoi(hrs[0])
		if err != nil {
			crylog.Error(INVALID_EXCLUDE_FORMAT_MESSAGE, err)
			return
		}
		hr2, err = strconv.Atoi(hrs[1])
		if err != nil {
			crylog.Error(INVALID_EXCLUDE_FORMAT_MESSAGE, err)
			return
		}
		if hr1 > 24 || hr1 < 0 || hr2 > 24 || hr1 < 0 {
			crylog.Error("INVALID_EXCLUDE_FORMAT_MESSAGE", ": XX and YY must each be between 0 and 24")
			return
		}
	}

	fmt.Printf("==== %s v%s ====\n", APPLICATION_NAME, VERSION_STRING)
	if *uname == DONATE_USERNAME {
		fmt.Printf("\nNo username specified, mining on behalf of donate.getmonero.org.\n")
	}
	if *saver {
		fmt.Printf("\nNOTE: Mining only when screen is locked. Specify -saver=false to mine always.\n")
	}
	if *t == 1 {
		fmt.Printf("\nMining with only one thread. Specify -threads=X to use more.\n")
		fmt.Printf("Or use the [i] keyboard command to add threads dynamically.\n")
	}
	if hr1 != -1 {
		fmt.Printf("\nMining will be paused between the hours of %v:00 and %v:00.\n", hr1, hr2)
	}
	fmt.Printf("\nMonitor your mining progress at: %s\n", STATS_WEBPAGE)
	fmt.Printf("\nSend feedback to: cryptonote.social@gmail.com\n")

	fmt.Println("\n==== Status/Debug output follows ====")
	crylog.Info("Miner username:", *uname)
	crylog.Info("Threads:", *t)

	if Mine(s, *t, *uname, *rigid, *saver, hr1, hr2, *startDiff, *tls, *config, agent) != nil {
		crylog.Error("Miner failed:", err)
	}
}
