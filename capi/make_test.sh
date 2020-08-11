go build -o capi.a -buildmode=c-archive capi.go

gcc -c test.c -o test.o
gcc test.o capi.a -lpthread -o test
