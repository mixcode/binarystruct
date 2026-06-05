// Copyright 2021-2026 github.com/mixcode

package binarystruct_test

import (
	"encoding"
	"fmt"

	"github.com/mixcode/binarystruct"
)

// WirePixel carries binary-layout tags and, by delegating to binarystruct with a
// fixed byte order, implements the standard library's encoding.BinaryMarshaler,
// encoding.BinaryUnmarshaler, and encoding.BinaryAppender (Go 1.24) interfaces.
//
// This is the recommended pattern for making a binarystruct-tagged type a
// first-class citizen of the stdlib encoding ecosystem (gob, your own framing,
// etc.): write these three small methods. (The codegen tool emits equivalent,
// reflection-free methods — see `binarystruct-codegen -endian big`.)
type WirePixel struct {
	_ struct{} `binary:"endian=big"` // declares the byte order; the twin inherits it
	X uint16   `binary:"uint16"`
	Y uint16   `binary:"uint16"`
}

// wirePixelLayout has WirePixel's exact memory layout and tags (including the
// endian sentinel) but NONE of its methods. Marshalling *this* type avoids
// infinite recursion: binarystruct honors encoding.BinaryMarshaler, so handing it
// a *WirePixel (which has MarshalBinary) would call MarshalBinary again, forever.
// Converting to a method-less twin breaks the cycle — the same trick encoding/json
// uses. Without it you get a stack overflow. (Codegen-generated methods sidestep
// this by writing fields directly.)
type wirePixelLayout WirePixel

// No order argument: the struct declares its byte order via the `_` sentinel.
func (p *WirePixel) MarshalBinary() ([]byte, error) {
	return binarystruct.Marshal((*wirePixelLayout)(p))
}

func (p *WirePixel) AppendBinary(b []byte) ([]byte, error) {
	return binarystruct.Append(b, (*wirePixelLayout)(p))
}

func (p *WirePixel) UnmarshalBinary(data []byte) error {
	_, err := binarystruct.Unmarshal(data, (*wirePixelLayout)(p))
	return err
}

// Ensure *WirePixel satisfies the three stdlib interfaces at compile time.
var (
	_ encoding.BinaryMarshaler   = (*WirePixel)(nil)
	_ encoding.BinaryAppender    = (*WirePixel)(nil)
	_ encoding.BinaryUnmarshaler = (*WirePixel)(nil)
)

func Example_encodingBinaryInterfaces() {
	p := &WirePixel{X: 0x0102, Y: 0x0304}

	// encoding.BinaryMarshaler
	data, _ := p.MarshalBinary()
	fmt.Printf("MarshalBinary:   % x\n", data)

	// encoding.BinaryAppender — append onto an existing buffer
	buf, _ := p.AppendBinary([]byte{0xff})
	fmt.Printf("AppendBinary:    % x\n", buf)

	// encoding.BinaryUnmarshaler — round-trip back
	var got WirePixel
	_ = got.UnmarshalBinary(data)
	fmt.Printf("UnmarshalBinary: {X:%d Y:%d}\n", got.X, got.Y)

	// Output:
	// MarshalBinary:   01 02 03 04
	// AppendBinary:    ff 01 02 03 04
	// UnmarshalBinary: {X:258 Y:772}
}
