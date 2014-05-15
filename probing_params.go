package fslm

import (
	"github.com/kho/word"
)

func WordIdHash(k word.Id) uint {
	// https://code.google.com/p/fast-hash
	h := uint64(k)
	h ^= h >> 23
	h *= 0x2127599bf4325c37
	h ^= h >> 47
	return uint(h)
}

func WordIdEqual(a, b word.Id) bool {
	return a == b
}
