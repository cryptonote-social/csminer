go build -o capi.a -buildmode=c-archive capi.go
gcc -c test.c -o test.o
gcc -L../../rxlib/ test.o capi.a ../../rxlib/rxlib.cpp.o -lpthread -lrandomx -lstdc++ -lm -o test
