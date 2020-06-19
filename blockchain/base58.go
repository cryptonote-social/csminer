// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package blockchain

// blockchain/base58.go implements "bitcoin style" base58 encoding/decoding of large ints

import (
	"fmt"
	"math/big"
)

var (
	alphabet [58]byte
	decoder  [256]int
	radix    *big.Int
	zero     *big.Int
)

func init() {
	alphabet = [58]byte{
		'1', '2', '3', '4', '5', '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E', 'F',
		'G', 'H', 'J', 'K', 'L', 'M', 'N', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W',
		'X', 'Y', 'Z', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}
	radix = big.NewInt(58)
	for i := range decoder {
		decoder[i] = -1
	}
	for i, b := range alphabet[:] {
		decoder[b] = i
	}
	radix = big.NewInt(58)
	zero = big.NewInt(0)
}

// given a bitcoin-base58 encoded string, returns the decoded string, or error if the input string
// was not bitcoin-base58 format
func DecodeBitcoinBase58(b58 string) (string, error) {
	if len(b58) == 0 {
		return "", nil
	}

	// handle leading zeros
	var zeros []byte
	for i := 0; i < len(b58)-1; i++ {
		if b58[i] != alphabet[0] {
			break
		}
		zeros = append(zeros, '0')
	}

	// now decode the rest
	bigOutput := new(big.Int)
	var decoded int
	for i := len(zeros); i < len(b58); i++ {
		c := b58[i]
		if decoded = decoder[c]; decoded < 0 {
			return "", fmt.Errorf("found invalid char [%c] while decoding base58 string [%v] at byte offset %v", c, b58, i)
		}
		bigOutput.Add(bigOutput.Mul(bigOutput, radix), big.NewInt(int64(decoded)))
	}
	return string(bigOutput.Append(zeros, 10)), nil
}

// given a base-10 integer string, returns the bitcoin-base58 encoded string of that integer, or
// error if the input was not a base-10 integer.
func EncodeBitcoinBase58(dec string) (string, error) {
	if len(dec) == 0 {
		return "", nil
	}
	bigInput, ok := new(big.Int).SetString(dec, 10)
	if !ok {
		return "", fmt.Errorf("expected a base10 string but got %s", dec)
	}

	// handle leading zeros
	output := []byte{}
	for i := 0; i < len(dec); i++ {
		if dec[i] != '0' {
			break
		}
		output = append(output, alphabet[0])
	}
	zeros := len(output)

	// now encode the rest
	var mod big.Int
	for bigInput.Cmp(zero) == 1 {
		bigInput.DivMod(bigInput, radix, &mod)
		output = append(output, alphabet[mod.Int64()])
	}
	reverse(output[zeros:])
	return string(output), nil
}

func reverse(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
