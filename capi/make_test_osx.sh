go build -o capi.dynlib -buildmode=c-shared capi.go
gcc -c test.c -o test.o
gcc test.o capi.dynlib -o test
