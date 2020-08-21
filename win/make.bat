del csminer.exe
del csminer_res.o
windres csminer.rc csminer_res.o
cd ..\..\rxlib
call make.bat
cd ..\csminer\win
ld -relocatable csminer_res.o ..\..\rxlib\rxlib.cpp.o -o ..\..\rxlib\rxlib.o
move ..\..\rxlib\rxlib.o ..\..\rxlib\rxlib.cpp.o
go build -a -ldflags="-s -w" csminer.go
