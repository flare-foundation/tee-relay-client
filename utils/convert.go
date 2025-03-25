package utils

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// ToBytes32 returns Solidity's bytes32(s) ([]byte(s) appended with zeros to length 32)
// String s can be at most 32 characters long, otherwise an error is returned.
func ToBytes32(s string) (common.Hash, error) {
	if len(s) > 32 {
		return common.Hash{}, fmt.Errorf("string %s too long. At most 32 characters allowed", s)
	}
	x := [32]byte{}
	copy(x[:], s)

	return x, nil
}
