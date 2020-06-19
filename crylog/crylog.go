// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

// Package crylog provides functionality for logging.
package crylog

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mu  sync.Mutex
	buf []byte   = make([]byte, 0)
	fd  *os.File = os.Stderr

	EXIT_ON_LOG_FATAL = flag.Bool(
		"exit-on-log-fatal", false, "whether to exit if a fatal error is logged")
)

func Info(v ...interface{}) {
	doLog("INFO", v)
}

func Warn(v ...interface{}) {
	doLog("WARN", v)
}

func Error(v ...interface{}) {
	doLog("ERROR", v)
}

func Fatal(v ...interface{}) {
	doLog("FATAL", v)
	if *EXIT_ON_LOG_FATAL {
		os.Exit(1)
	}
}

func SetOutput(filePath string) error {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0664)
	if err != nil {
		return err
	}
	fd = f
	return nil
}

// formatFileAndLine is a helper func that returns a formatted string containing the filename and
// line number of where the logging call was invoked from.
func formatFileAndLine(buf *[]byte, depth int) {
	_, f, l, ok := runtime.Caller(depth)
	if !ok {
		println("internal logging error")
		*buf = append(*buf, "()"...)
	}
	if i := strings.LastIndex(f, "/"); i != -1 {
		f = f[i+1:]
	}
	*buf = append(*buf, '(')
	*buf = append(*buf, f...)
	*buf = append(*buf, ',')
	*buf = append(*buf, strconv.Itoa(l)...)
	*buf = append(*buf, ')')
}

func doLog(prefix string, v []interface{}) {
	now := time.Now()
	mu.Lock()
	defer mu.Unlock()
	buf = buf[:0]
	formatHeader(&buf, now)
	buf = append(buf, prefix...)
	formatFileAndLine(&buf, 3)
	buf = append(buf, ": "...)
	buf = append(buf, fmt.Sprintln(v...)...)
	_, err := fd.Write(buf)
	if err != nil {
		println("logging error")
	}
}

func itoa(buf *[]byte, i int, wid int) {
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}

func formatHeader(buf *[]byte, t time.Time) {
	*buf = append(*buf, "# "...)
	year, month, day := t.Date()
	itoa(buf, year, 4)
	*buf = append(*buf, '/')
	itoa(buf, int(month), 2)
	*buf = append(*buf, '/')
	itoa(buf, day, 2)
	*buf = append(*buf, ' ')
	hour, min, sec := t.Clock()
	itoa(buf, hour, 2)
	*buf = append(*buf, ':')
	itoa(buf, min, 2)
	*buf = append(*buf, ':')
	itoa(buf, sec, 2)
	*buf = append(*buf, ' ')
}
