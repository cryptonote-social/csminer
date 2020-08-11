go build -o capi.dynlib -buildmode=c-shared -ldflags="-extldflags=-Wl,-install_name,@rpath/capi.dynlib" capi.go
gcc -c test.c -o test.o
gcc test.o capi.dynlib -rpath `pwd` -o test
