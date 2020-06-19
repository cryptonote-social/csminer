package crylog

import (
	"testing"
)

// These test cases don't actually check output yet, but can be used to visually inspect output
// is as expected with go test -v.

func TestInfoLog(t *testing.T) {
	Info("this is an info logging test")
}

func TestWarnLog(t *testing.T) {
	Warn("this is a warn logging test")
}

func TestErrorLog(t *testing.T) {
	Error("this is an error logging test")
}

func TestFatalLog(t *testing.T) {
	exit := false
	EXIT_ON_LOG_FATAL = &exit
	Fatal("this is a fatal logging test")
	// TODO: Test that os.exit() is called when EXIT_ON_LOG_FATAL is true
	// exit = true
	// Fatal("this is a fatal logging test")
}
