del csminer.exe
del csminer_res.o
windres csminer.rc csminer_res.o
cd ..\..\RandomX\rxlib
call make.bat
cd ..\..\csminer\win
ld -relocatable csminer_res.o ..\..\RandomX\rxlib\rxlib.cpp.o -o ..\..\RandomX\rxlib\rxlib.o
move ..\..\RandomX\rxlib\rxlib.o ..\..\RandomX\rxlib\rxlib.cpp.o
go build -a -ldflags="-s -w" csminer.go
