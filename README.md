Csminer (cryptonote.social miner) is an easy-to-use command-line miner for Monero providing "set it and forget it" background mining for your personal laptop and desktop machines. Above all, csminer tries to keep day to day usage of your machine unaffected while it is running. Once started, you'll find there's no need to have to stop it to use your machine for other tasks, and then remember to manually restart when done. And you should still find its hashrate comparable to other Monero mining software.

By default, csminer will mine with a single thread, and only when your screen is inactive. It also mines only while on AC power, making it suitable even for laptops. You can also have csminer pause mining during certain hours of the day, for example to avoid periods of higher electricity rates or higher expected machine usage. All of these options are of course easily configurable if you wish to mine more aggressively! 

Project uses CGO and relays on https://github.com/cryptonote-social/RandomX.git.

## Install
https://cryptonote.social/tools/csminer

## Build
Install dependacies
```sh
apt update && apt install git make cmake gcc g++
```

### Linux
```sh
git clone https://github.com/cryptonote-social/RandomX.git && \
git clone https://github.com/cryptonote-social/csminer.git && \
mkdir -p RandomX/build && cd RandomX/build/ && \
cmake .. && make && \
cd ../rxlib && ./make.sh && \
cd ../../csminer/ && \
go build linux/csminer.go
```

### OSX
```sh
git clone https://github.com/cryptonote-social/RandomX.git && \
git clone https://github.com/cryptonote-social/csminer.git && \
mkdir -p RandomX/build && cd RandomX/build/ && \
cmake .. && make && \
cd ../rxlib && ./make.sh && \
cd ../../csminer/ && \
go build osx/csminer.go
```

### Windows
```ps
...
```

### Linux (ARM)
```sh
...
```
