// Copyright 2026 github.com/mixcode

//go:build safe_binarystruct || !amd64 || !go1.26 || !experiment_simd

package binarystruct

// Scalar byte-swap fallback for every build the amd64 SIMD kernel does not cover.
//
// simd/archsimd is amd64-only in Go 1.26, so only amd64 has a vectorized kernel
// (simd_amd64.go). If upstream adds another architecture (arm64 NEON is the
// likely first), implement simdSwap16/32/64 for it in a new simd_<arch>.go using
// that ISA's primitives (e.g. NEON REV16/REV32/REV64) and remove the arch from
// this file's build-tag negation. See the SIMD upstream-tracking note in
// AGENTS.txt. The swapBytes dispatcher and swap_test.go are arch-agnostic.

func simdSwap16(buf []byte) {
	for i := 0; i < len(buf); i += 2 {
		buf[i], buf[i+1] = buf[i+1], buf[i]
	}
}

func simdSwap32(buf []byte) {
	for i := 0; i < len(buf); i += 4 {
		_ = buf[i+3] // bounds check elimination
		buf[i], buf[i+1], buf[i+2], buf[i+3] = buf[i+3], buf[i+2], buf[i+1], buf[i]
	}
}

func simdSwap64(buf []byte) {
	for i := 0; i < len(buf); i += 8 {
		_ = buf[i+7] // bounds check elimination
		buf[i], buf[i+1], buf[i+2], buf[i+3], buf[i+4], buf[i+5], buf[i+6], buf[i+7] =
			buf[i+7], buf[i+6], buf[i+5], buf[i+4], buf[i+3], buf[i+2], buf[i+1], buf[i]
	}
}
