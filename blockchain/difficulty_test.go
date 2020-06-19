// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package blockchain

import (
	"testing"
)

type hashPair struct {
	minerDifficulty int64
	hashTarget      string
	inverseTarget   int64
}

var goodHashPairs = []hashPair{
	{100000000, "2a000000", 102261126},
	{400000, "f1290000", 400015},
	{2000, "9bc42000", 2000},
	{100, "285c8f02", 100},
	{1, "ffffffff", 1},
	{0x7FFFFFFF, "02000000", 0x7FFFFFFF},
	{0xFFFFFFFF, "01000000", 0xFFFFFFFF},
	{0, "", 0},
}

func TestHashTarget(t *testing.T) {
	for _, test := range goodHashPairs {
		target := HashTarget(test.minerDifficulty)
		if target != test.hashTarget {
			t.Errorf("expected %v for HashTarget(%v), got %v", test.hashTarget, test.minerDifficulty, target)
		}
		diff := TargetToDifficulty(target)
		if diff != test.inverseTarget {
			t.Errorf("expected %v for TargetToDifficulty(%v), got %v", test.inverseTarget, target, diff)
		}
		rounded := RoundDifficulty(test.minerDifficulty)
		if rounded != test.inverseTarget {
			t.Errorf("expected %v for RoundDifficulty(%v), got %v", test.inverseTarget, test.minerDifficulty, rounded)
		}
	}
}
