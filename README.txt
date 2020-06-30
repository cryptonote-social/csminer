csminer v0.0.8

SYNOPSIS

csminer from https://cryptonote.social is an easy-to-use CPU miner for Monero intended to provide
"set it and forget it" mining for your existing laptop and desktop machines. By default, csminer
mines with a single thread, and only when your screen is locked. It can be configured to always
mine or mine with more threads using the options described below. If you don't specify a username,
csminer will mine on behalf of the Monero project (https://donate.getmonero.org) with earnings
going directly to its wallet:

888tNkZrPN6JsEgekjMnABU4TBzc2Dt29EPAvkRxbANsAnjyPbb3iQ1YBRk1UXcdRsiKc9dhwMVgN5S9cQUiyoogDavup3H


USAGE

./csminer [OPTION]...

All arguments are optional:
  -user <string>
    	your pool username from https://cryptonote.social/xmr (default "donate-getmonero-org")
  -saver <bool>
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
  -start_diff <int>
        starting difficulty value for the pool

Monitor your miner progress at: https://cryptonote.social/xmr


ADDITIONAL DETAILS

Usernames

If you don't already have a username established with the pool, you can create one by simply
specifying -user=your-wallet-id.your-chosen-username, e.g.:

./csminer -user=888tNkZrPN6JsEgekjMnABU4TBzc2Dt29EPAvkRxbANsAnjyPbb3iQ1YBRk1UXcdRsiKc9dhwMVgN5S9cQUiyoogDavup3H.donate-monero-org

Once you've submitted at least one share, the wallet address is permanently associated with the
username and you can login with the username alone, e.g.:

./csminer -user=get-monero-org

Implementation

csminer utilizes the RandomX implementation from https://github.com/tevador/RandomX and will
perform similarly to other mining software based on this library such as xmrig. It will attempt to
use hugepages if available, so for optimal performance confirm hugepages is enabled on your
machine. If hugepages is enabled but the miner reports it's unable to allocate them, try restarting
your machine.

Windows version: csminer for Windows monitors session notifications for session lock messages and
should activate the miner whenever the screen is locked.

Mac/OSX version: csminer for OSX polls the lock screen state every 10 seconds and should activate
the miner shortly after the screen locks. Power napping should be disabled to ensure the miner
isn't suspended while the screen is locked.

Linux/Gnome version: csminer for Linux monitors Gnome screensaver events, and should activate the miner
whenever the screen "dims".


Feedback & Bug Reports

Send to: <cryptonote.social@gmail.com>
