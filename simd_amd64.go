// Copyright 2026 github.com/mixcode

//go:build !safe_binarystruct && amd64 && go1.26 && experiment_simd

package binarystruct

import (
	"simd/archsimd"
)

// Byte-swap shuffle indices for VPSHUFB (PermuteOrZeroGrouped). The op applies
// the low 4 bits of each index byte within its own 128-bit group, so the same
// 16-byte pattern is repeated across both lanes of the 256-bit vector. Reversing
// each element in place is therefore a single within-lane shuffle.
var (
	swap16Idx = [32]int8{
		1, 0, 3, 2, 5, 4, 7, 6, 9, 8, 11, 10, 13, 12, 15, 14,
		1, 0, 3, 2, 5, 4, 7, 6, 9, 8, 11, 10, 13, 12, 15, 14,
	}
	swap32Idx = [32]int8{
		3, 2, 1, 0, 7, 6, 5, 4, 11, 10, 9, 8, 15, 14, 13, 12,
		3, 2, 1, 0, 7, 6, 5, 4, 11, 10, 9, 8, 15, 14, 13, 12,
	}
	swap64Idx = [32]int8{
		7, 6, 5, 4, 3, 2, 1, 0, 15, 14, 13, 12, 11, 10, 9, 8,
		7, 6, 5, 4, 3, 2, 1, 0, 15, 14, 13, 12, 11, 10, 9, 8,
	}
)

func simdSwap16(buf []byte) {
	i := 0
	if archsimd.X86.AVX2() && len(buf) >= 32 {
		idx := archsimd.LoadInt8x32(&swap16Idx)
		for ; i+32 <= len(buf); i += 32 {
			archsimd.LoadUint8x32Slice(buf[i:]).PermuteOrZeroGrouped(idx).StoreSlice(buf[i:])
		}
	}
	for ; i < len(buf); i += 2 {
		buf[i], buf[i+1] = buf[i+1], buf[i]
	}
}

func simdSwap32(buf []byte) {
	i := 0
	if archsimd.X86.AVX2() && len(buf) >= 32 {
		idx := archsimd.LoadInt8x32(&swap32Idx)
		for ; i+32 <= len(buf); i += 32 {
			archsimd.LoadUint8x32Slice(buf[i:]).PermuteOrZeroGrouped(idx).StoreSlice(buf[i:])
		}
	}
	for ; i < len(buf); i += 4 {
		_ = buf[i+3] // bounds check elimination
		buf[i], buf[i+1], buf[i+2], buf[i+3] = buf[i+3], buf[i+2], buf[i+1], buf[i]
	}
}

func simdSwap64(buf []byte) {
	i := 0
	if archsimd.X86.AVX2() && len(buf) >= 32 {
		idx := archsimd.LoadInt8x32(&swap64Idx)
		for ; i+32 <= len(buf); i += 32 {
			archsimd.LoadUint8x32Slice(buf[i:]).PermuteOrZeroGrouped(idx).StoreSlice(buf[i:])
		}
	}
	for ; i < len(buf); i += 8 {
		_ = buf[i+7] // bounds check elimination
		buf[i], buf[i+1], buf[i+2], buf[i+3], buf[i+4], buf[i+5], buf[i+6], buf[i+7] =
			buf[i+7], buf[i+6], buf[i+5], buf[i+4], buf[i+3], buf[i+2], buf[i+1], buf[i]
	}
}
