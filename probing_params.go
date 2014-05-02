package fslm

func WordIdHash(k WordId) uint {
	// https://code.google.com/p/fast-hash
	h := uint64(k)
	h ^= h >> 23
	h *= 0x2127599bf4325c37
	h ^= h >> 47
	return uint(h)
}

func WordIdEqual(a, b WordId) bool {
	return a == b
}
