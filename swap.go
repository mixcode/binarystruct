// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"encoding/binary"
	"unsafe"
)

// hostEndian is the machine's native byte order, detected once at init. The
// runtime unsafe path and generated bulk-slice code both compare a field's
// requested order against it to decide whether a byte-swap is needed.
var hostEndian binary.ByteOrder

func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)
	if buf[0] == 0xAB {
		hostEndian = binary.BigEndian
	} else {
		hostEndian = binary.LittleEndian
	}
}

// HostEndian reports the machine's native byte order. Generated code uses it to
// decide whether a bulk scalar slice read/written as raw memory needs swapping:
// when the requested order equals HostEndian(), the raw bytes are already
// correct and no swap is performed.
func HostEndian() ByteOrder { return hostEndian }

// SwapBytes reverses the byte order of every width-byte element of buf in place.
// width must be 2, 4, or 8 (any other value is a no-op); len(buf) is expected to
// be a multiple of width. It backs the bulk scalar-slice path emitted by
// binarystruct-codegen and is accelerated by the SIMD kernel when built with
// `-tags experiment_simd` (GOEXPERIMENT=simd) on amd64; otherwise it is the
// scalar fallback. See simd_amd64.go / simd_fallback.go.
func SwapBytes(buf []byte, width int) { swapBytes(buf, width) }

func swapBytes(buf []byte, sz int) {
	switch sz {
	case 2:
		simdSwap16(buf)
	case 4:
		simdSwap32(buf)
	case 8:
		simdSwap64(buf)
	}
}
