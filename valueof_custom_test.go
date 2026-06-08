// Copyright 2021-2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"errors"
	"hash/crc32"
	"testing"
)

// A PNG-chunk-shaped layout that exercises a custom valueof evaluator: the
// 4-byte length is derived with the built-in bytelen(Data), and the trailing
// CRC is derived with a registered CRC32(Type, Data) evaluator over the encoded
// bytes of the two covered fields. Byte order is declared on the struct.
type crcChunk struct {
	_      struct{} `binary:"endian=big"`
	Length uint32   `binary:"uint32,valueof=bytelen(Data)"`
	Type   string   `binary:"string(4)"`
	Data   []byte   `binary:"[Length]byte"`
	CRC    uint32   `binary:"uint32,valueof=CRC32(Type, Data)"`
}

func newCRCMarshaler() *Marshaler {
	ms := NewMarshaler()
	ms.AddValueOf("CRC32", func(c ValueOfContext) (uint64, error) {
		h := crc32.NewIEEE()
		for _, a := range c.Args {
			h.Write(a.Bytes)
		}
		return uint64(h.Sum32()), nil
	})
	return ms
}

func TestCustomValueof_PNGChunkRoundTrip(t *testing.T) {
	ms := newCRCMarshaler()
	in := crcChunk{Type: "IHDR", Data: []byte{1, 2, 3, 4, 5}}

	blob, err := ms.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Independent cross-check: the CRC the library wrote must equal a fresh
	// crc32(type||data), and the length must equal len(Data).
	wantCRC := crc32.ChecksumIEEE(append([]byte("IHDR"), in.Data...))
	// layout: [4]len [4]type [5]data [4]crc = 17 bytes, all big-endian.
	if len(blob) != 4+4+5+4 {
		t.Fatalf("encoded length = %d, want 17; blob=% x", len(blob), blob)
	}
	gotLen := uint32(blob[0])<<24 | uint32(blob[1])<<16 | uint32(blob[2])<<8 | uint32(blob[3])
	if gotLen != 5 {
		t.Errorf("encoded Length = %d, want 5", gotLen)
	}
	gotCRC := uint32(blob[13])<<24 | uint32(blob[14])<<16 | uint32(blob[15])<<8 | uint32(blob[16])
	if gotCRC != wantCRC {
		t.Errorf("encoded CRC = %#08x, want %#08x", gotCRC, wantCRC)
	}

	// Decode: validation must pass for well-formed bytes.
	var out crcChunk
	if _, err := ms.Unmarshal(blob, &out); err != nil {
		t.Fatalf("unmarshal valid chunk: %v", err)
	}
	if out.Type != "IHDR" || !bytes.Equal(out.Data, in.Data) {
		t.Errorf("round trip mismatch: got %+v", out)
	}
}

func TestCustomValueof_DecodeRejectsBadCRC(t *testing.T) {
	ms := newCRCMarshaler()
	blob, err := ms.Marshal(&crcChunk{Type: "IHDR", Data: []byte{1, 2, 3, 4, 5}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Corrupt the last CRC byte.
	corrupt := append([]byte(nil), blob...)
	corrupt[len(corrupt)-1] ^= 0xff

	var out crcChunk
	_, err = ms.Unmarshal(corrupt, &out)
	if err == nil {
		t.Fatal("expected a validation error on a corrupted CRC, got nil")
	}
	if !errors.Is(err, ErrValidationError) {
		t.Errorf("error %q does not wrap ErrValidationError", err)
	}
	var de *DecodeError
	if errors.As(err, &de) {
		if de.Field != "CRC" {
			t.Errorf("DecodeError.Field = %q, want CRC", de.Field)
		}
	} else {
		t.Errorf("error is not a *DecodeError: %v", err)
	}
}

func TestCustomValueof_UnregisteredEvaluatorErrors(t *testing.T) {
	// A Marshaler without the CRC32 evaluator registered must fail loud rather
	// than silently writing a zero.
	ms := NewMarshaler()
	_, err := ms.Marshal(&crcChunk{Type: "IHDR", Data: []byte{1, 2}})
	if err == nil {
		t.Fatal("expected an error for an unregistered valueof evaluator, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("CRC32")) {
		t.Errorf("error %q should name the missing evaluator CRC32", err)
	}
}
