del csminer.exe
del csminer_res.o
del ..\rx\cpp\rxlib.cpp.o
del ..\rx\cpp\rxlib.o
cd ..\rx\cpp
call make.bat
cd ..\..\win
windres csminer.rc csminer_res.o
ld -relocatable csminer_res.o ..\rx\cpp\rxlib.cpp.o -o ..\rx\cpp\rxlib.o
move ..\rx\cpp\rxlib.o ..\rx\cpp\rxlib.cpp.o
go build -x -ldflags="-s -w" csminer.go
