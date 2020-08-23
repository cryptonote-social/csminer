go build -a -o capi.dynlib -buildmode=c-shared -ldflags="-extldflags=-Wl,-install_name,@rpath/capi.dynlib -s -w" capi.go

gcc -O3 -c test.c -o test.o
gcc -O3 -L../../rxlib/ test.o capi.dynlib ../../rxlib/rxlib.cpp.o -lrandomx -lstdc++ -lm -rpath `pwd` -o test
