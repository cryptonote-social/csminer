csminer v0.3.1 (Linux/Gnome version)

SYNOPSIS

csminer from https://cryptonote.social is an easy-to-use CPU miner for Monero intended to provide
"set it and forget it" mining for your existing laptop and desktop machines. By default, csminer
mines with a single thread, and only when Gnome dims the screen. It can be configured to always
mine or mine with more threads using the options described below.


USAGE

./csminer [OPTION]...

All arguments are optional:
  -user <string>
        your pool username from https://cryptonote.social/xmr (default "donate-getmonero-org")
  -saver=<bool>
        mine only when screen is dimmed (default true)
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
        Started on the pool site for details. Some options will require you to also specify your
        wallet id (see below) in order to be changed.
  -wallet <string>
        your wallet id. You only need to specify this when establishing a new username, or if
        specifying a 'secure' config parameter change such as a new pool donation amount or email
        address. New usernames will be established upon submitting at least one valid share.

Monitor your miner progress at: https://cryptonote.social/xmr, or type <p> + <enter> to display
pool stats in the command shell.

Tips:

For best performance, if csminer reports it's unable to allocate hugepages, try restarting your
machine and starting csminer before running anything else.

If you don't specify a username, csminer will mine on behalf of the Monero project
(https://donate.getmonero.org) with earnings going directly to its wallet:

888tNkZrPN6JsEgekjMnABU4TBzc2Dt29EPAvkRxbANsAnjyPbb3iQ1YBRk1UXcdRsiKc9dhwMVgN5S9cQUiyoogDavup3H


ADDITIONAL DETAILS

Usernames

If you don't already have a username established with the pool, you can create one by simply
specifying your wallet id with the -wallet option along with your selected username, e.g.:

./csminer -user=your-chosen-username -wallet=your-wallet-id

Once you've submitted at least one share, the wallet address is permanently associated with the
username and you can login with the username alone, e.g.:

./csminer -user=your-chosen-username

Wallet address must be specified however if you want to send authenticated chat messages or
make config updates such as changing your notification e-mail address.

Implementation

csminer utilizes the RandomX implementation from https://github.com/tevador/RandomX and will
perform similarly to other mining software based on this library such as xmrig.

Feedback & Bug Reports

Send to: <cryptonote.social@gmail.com>, or send a chat to the build-in chatroom using the "c <chat
message>" key command.
