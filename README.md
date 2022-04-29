## Build
Make sure you hava `go`, `cmake` and `make` installed.

Linux
```sh
git clone https://github.com/cryptonote-social/RandomX.git && \
git clone https://github.com/cryptonote-social/csminer.git && \
mkdir -p RandomX/build && cd RandomX/build/ && \
cmake .. && make && \
cd ../rxlib && ./make.sh && \
cd ../../csminer/ && \
go build linux/csminer.go && ./csminer
```

OSX
```sh
git clone https://github.com/cryptonote-social/RandomX.git && \
git clone https://github.com/cryptonote-social/csminer.git && \
mkdir -p RandomX/build && cd RandomX/build/ && \
cmake .. && make && \
cd ../rxlib && ./make.sh && \
cd ../../csminer/ && \
go build osx/csminer.go && ./csminer
```

Windows
```ps
...
```