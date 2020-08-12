package main

import "C"

import (
	"github.com/cryptonote-social/csminer/minerlib"
)

//export PoolLogin
func PoolLogin(
	username *C.char,
	rigid *C.char,
	wallet *C.char,
	agent *C.char,
	config *C.char) (
	code int,
	message *C.char) {
	args := &minerlib.PoolLoginArgs{
		Username: C.GoString(username),
		RigID:    C.GoString(rigid),
		Wallet:   C.GoString(wallet),
		Agent:    C.GoString(agent),
		Config:   C.GoString(config),
	}
	resp := minerlib.PoolLogin(args)
	return resp.Code, C.CString(resp.Message)
}

//export InitMiner
func InitMiner(threads int, excludeHrStart, excludeHrEnd int) (code int, message *C.char) {
	args := &minerlib.InitMinerArgs{
		Threads:          threads,
		ExcludeHourStart: excludeHrStart,
		ExcludeHourEnd:   excludeHrEnd,
	}
	resp := minerlib.InitMiner(args)
	return resp.Code, C.CString(resp.Message)
}

func main() {}
