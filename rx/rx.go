// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

// Package rx provides Go access to various randomx library methods.
package rx

// #cgo CFLAGS: -std=c11 -D_GNU_SOURCE -m64
// #cgo LDFLAGS: -L${SRCDIR}/cpp/ -Wl,-rpath,$ORIGIN ${SRCDIR}/cpp/rxlib.cpp.o -lrandomx -lstdc++ -lm
/*
 #include <stdlib.h>
 #include "cpp/rxlib.h"
*/
import "C"

import (
	//	"cryptonote.social/crylog"
	//	"encoding/hex"
	"unsafe"
)

// Call this every time the seed hash provided by the daemon changes before performing any hashing.
func InitRX(seedHash []byte, threads int, initThreads int) bool {
	if len(seedHash) == 0 {
		return false
	}
	b := C.init_rxlib(
		(*C.char)(unsafe.Pointer(&seedHash[0])),
		(C.uint32_t)(len(seedHash)),
		(C.int)(threads),
		(C.int)(initThreads))
	return bool(b)
}

func HashUntil(blob []byte, difficulty uint64, thread int, hash []byte, nonce []byte, stopper *uint32) int64 {
	res := C.rx_hash_until(
		(*C.char)(unsafe.Pointer(&blob[0])),
		(C.uint32_t)(len(blob)),
		(C.uint64_t)(difficulty),
		(C.int)(thread),
		(*C.char)(unsafe.Pointer(&hash[0])),
		(*C.char)(unsafe.Pointer(&nonce[0])),
		(*C.uint32_t)(unsafe.Pointer(stopper)))
	return int64(res)
}
