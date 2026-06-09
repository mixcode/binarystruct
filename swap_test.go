// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"testing"
)

// refSwap reverses each sz-byte element, independent of the production code.
func refSwap(buf []byte, sz int) []byte {
	out := make([]byte, len(buf))
	copy(out, buf)
	for i := 0; i+sz <= len(out); i += sz {
		for a, b := i, i+sz-1; a < b; a, b = a+1, b-1 {
			out[a], out[b] = out[b], out[a]
		}
	}
	return out
}

// TestSwapBytes checks swapBytes (and thus simdSwap16/32/64) against an
// independent reference across element sizes and lengths that straddle the
// 32-byte vector chunk boundary and leave assorted tails — the cases the
// scalar-only path never exercised.
func TestSwapBytes(t *testing.T) {
	for _, sz := range []int{2, 4, 8} {
		// element counts spanning <1 vector, exact multiples, and odd tails.
		for n := 0; n <= 40; n++ {
			buf := make([]byte, n*sz)
			for i := range buf {
				buf[i] = byte(i*7 + 1) // deterministic, non-trivial
			}
			want := refSwap(buf, sz)
			swapBytes(buf, sz)
			if !bytes.Equal(buf, want) {
				t.Fatalf("swapBytes(sz=%d, len=%d) mismatch:\n got %v\nwant %v", sz, len(buf), buf, want)
			}
		}
	}
}
