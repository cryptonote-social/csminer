// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package blockchain

// blockchain/difficulty.go implements various utility functions related to proof of work and
// difficulty testing.

import (
	"encoding/hex"
	"github.com/cryptonote-social/csminer/crylog"
	"math"
	"math/big"
)

var (
	maxTarget big.Int
	bigInt64  big.Int
)

func init() {
	// 32 bytes of 0xFF
	maxTarget.SetString("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 0)
	bigInt64.SetString("0xFFFFFFFF", 0)
}

// HashTarget converts "miner difficulty" into the 4-byte "hash target", returning it in hex format
func HashTarget(minerDifficulty int64) string {
	if minerDifficulty == 0 {
		return ""
	}
	bigInt := new(big.Int).Div(&bigInt64, big.NewInt(minerDifficulty))
	buf := bigInt.Bytes()
	reverse(buf)
	buf2 := make([]byte, 4, 4)
	copy(buf2, buf)
	return hex.EncodeToString(buf2)
}

// "Round" difficulty to a miner-supported boundary (due to how "target" is encoded, we don't have
// full integer fidelity of difficulty values).
func RoundDifficulty(unroundedDifficulty int64) int64 {
	if unroundedDifficulty == 0 {
		return 0
	}
	bigguns := new(big.Int).Div(&bigInt64, big.NewInt(unroundedDifficulty))
	mul := new(big.Int).Div(&bigInt64, bigguns)
	return mul.Int64()
}

func TargetToDifficulty(target string) int64 {
	byteTarget, err := hex.DecodeString(target)
	if err != nil {
		crylog.Error("Couldn't decode hex:", err)
		return 0
	}
	reverse(byteTarget)
	bigguns := new(big.Int).SetBytes(byteTarget)
	if bigguns.Int64() == 0 {
		return 0
	}
	mul := new(big.Int).Div(&bigInt64, bigguns)
	return mul.Int64()
}

// HashDifficulty takes a miner-provided blob hash and converts it to its difficulty
func HashDifficulty(hash []byte) int64 {
	var diff big.Int
	hashBytes := make([]byte, len(hash))
	copy(hashBytes, hash)
	reverse(hashBytes)
	diff.SetBytes(hashBytes)
	if diff.Int64() == 0 {
		return 0
	}
	diff.Div(&maxTarget, &diff)
	if diff.IsInt64() {
		return diff.Int64()
	}
	crylog.Error("should this happen????", diff)
	return math.MaxInt64
}
