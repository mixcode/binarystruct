// Copyright 2026 github.com/mixcode

//go:build safe || !amd64 || !go1.26 || !experiment_simd

package binarystruct

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
