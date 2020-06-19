// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package blockchain

import (
	"testing"
)

type pair struct {
	dec string
	b58 string
}

// valid {decimal, base58} pairs
var goodTestcases = []pair{
	{"", ""},
	{"0", "1"},
	{"00", "11"},
	{"000", "111"},
	{"3204985730948752093487", "3DGYQFvbohbyQ"},
	{"03204985730948752093487", "13DGYQFvbohbyQ"},
	{"003204985730948752093487", "113DGYQFvbohbyQ"},
	{"579329456446382921696683018105639567301940873444678852915136782360170728285027137130275902830063317377116645435602446493283240942461504392400127033691719162997615194905084", "WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJP"},
}

var badEncodeTestcases = []string{
	"NOT_A_GOOD_STRING",
	// embed letter O at various places in an otherwise valid decimal string:
	"O3204985730948752093487",
	"32049857O30948752093487",
	"3204985730948752093487O",
}

var badDecodeTestcases = []string{
	"NOT_A_GOOD_STRING",
	// embed 0 and O and various places in an otherwise valid base58 string:
	"OWmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJP",
	"WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJPO",
	"WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzOE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJP",
	"0WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2oe772rmZiqgJP",
	"WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjzE3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJP0",
	"WmtyffBcPhRDXi8ARVPvijY6ynshku5EcZwPHJUNjz0E3GfjmbUajD2ZBgLz4ghvtg4NdtHDQFYdz2WWy4omL2e772rmZiqgJP",
}

func TestEncodeDecode(t *testing.T) {
	for _, test := range goodTestcases {
		got, err := EncodeBitcoinBase58(test.dec)
		if err != nil {
			t.Errorf("error while encoding %s: %v", test.dec, err)
		} else if got != test.b58 {
			t.Errorf("expected %v, got %v while encoding %s", test.b58, got, test.dec)
		}

		got, err = DecodeBitcoinBase58(test.b58)
		if err != nil {
			t.Errorf("error while decoding %s: %v", test.b58, err)
		} else if string(got) != test.dec {
			t.Errorf("expected %v, got %v while decoding %s", test.dec, got, test.b58)
		}
	}

	for _, bad := range badEncodeTestcases {
		got, err := EncodeBitcoinBase58(bad)
		if err == nil {
			t.Errorf("expected error, got %s while encoding %s", got, bad)
		}
	}
	for _, bad := range badDecodeTestcases {
		got, err := DecodeBitcoinBase58(bad)
		if err == nil {
			t.Errorf("expected error, got %s while decoding %s", got, bad)
		}
	}
}
