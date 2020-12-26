package main

import "C"

import (
	"github.com/cryptonote-social/csminer/minerlib"
	"github.com/cryptonote-social/csminer/minerlib/chat"
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

//export GetMinerState
func GetMinerState() (
	miningActivity int,
	threads int,
	recentHashrate float64,
	username *C.char,
	secondsOld int,
	lifetimeHashes int64,
	paid, owed, accumulated float64,
	timeToReward *C.char,
	chatsAvailable bool) {
	resp := minerlib.GetMiningState()

	return resp.MiningActivity, resp.Threads, resp.RecentHashrate,
		C.CString(resp.PoolUsername), resp.SecondsOld, resp.LifetimeHashes,
		resp.Paid, resp.Owed, resp.Accumulated, C.CString(resp.TimeToReward),
		resp.ChatsAvailable
}

//export NextChat
func NextChat() (
	username *C.char,
	message *C.char,
	id int64,
	timestamp int64) {
	nc := chat.NextChatReceived()
	if nc == nil {
		return C.CString(""), C.CString(""), 0, 0
	}
	return C.CString(nc.Username), C.CString(nc.Message), nc.ID, nc.Timestamp
}

//export SendChat
func SendChat(message *C.char) int64 {
	return chat.SendChat(C.GoString(message))
}

//export IncreaseThreads
func IncreaseThreads() {
	minerlib.IncreaseThreads()
}

//export DecreaseThreads
func DecreaseThreads() {
	minerlib.DecreaseThreads()
}

//export OverrideMiningActivityState
func OverrideMiningActivityState(mine bool) {
	minerlib.OverrideMiningActivityState(mine)
}

//export RemoveMiningActivityOverride
func RemoveMiningActivityOverride() {
	minerlib.RemoveMiningActivityOverride()
}

//export ReportLockScreenState
func ReportLockScreenState(locked bool) {
	minerlib.ReportIdleScreenState(locked)
}

//export ReportPowerState
func ReportPowerState(onBattery bool) {
	minerlib.ReportPowerState(onBattery)
}

func main() {}
