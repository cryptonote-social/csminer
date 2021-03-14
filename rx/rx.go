// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

// Package rx provides Go access to various randomx library methods.
package rx

// #cgo CFLAGS: -std=c11 -D_GNU_SOURCE -m64 -O3 -I${SRCDIR}/../../RandomX/rxlib/
// #cgo LDFLAGS: -L${SRCDIR}/../../RandomX/rxlib/ -Wl,-rpath,$ORIGIN ${SRCDIR}/../../RandomX/rxlib/rxlib.cpp.o -lrandomx -lstdc++ -lm
/*
 #include <stdlib.h>
 #include "rxlib.h"
*/
import "C"

import (
	"github.com/cryptonote-social/csminer/crylog"
	//	"encoding/hex"
	"unsafe"
)

// Call this every time the seed hash provided by the daemon changes before performing any hashing.
// Only call when all existing threads are stopped. Returns false if an unrecoverable error
// occurred.
func SeedRX(seedHash []byte, initThreads int) bool {
	if len(seedHash) == 0 {
		crylog.Error("Bad seed hash:", seedHash)
		return false
	}
	b := C.seed_rxlib(
		(*C.char)(unsafe.Pointer(&seedHash[0])),
		(C.uint32_t)(len(seedHash)),
		(C.int)(initThreads))
	return bool(b)
}

// Call this once.
// return values:
//   1: success
//   2: success, but no huge pages.
//   -1: unexpected failure
func InitRX(threads int) int {
	i := C.init_rxlib((C.int)(threads))
	return int(i)
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

// only call when all existing threads are stopped
func AddThread() int {
	res := C.rx_add_thread()
	return int(res)
}

// only call when all existing threads are stopped
func RemoveThread() int {
	res := C.rx_remove_thread()
	return int(res)
}
