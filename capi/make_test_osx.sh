go build -a -ldflags="-s -w" -o capi.dynlib -buildmode=c-shared -ldflags="-extldflags=-Wl,-install_name,@rpath/capi.dynlib" capi.go

gcc -c test.c -o test.o
gcc  -L../../rxlib/ test.o capi.dynlib ../../rxlib/rxlib.cpp.o -lrandomx -lstdc++ -lm -rpath `pwd` -o test
