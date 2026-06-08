// Copyright 2026 github.com/mixcode

package example

import (
	"bytes"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/mixcode/binarystruct"
)

// TestPacket round-trips a Packet through the no-arg encoding.BinaryMarshaler
// methods. Magic is a const (written automatically, validated on decode) and
// Version is range-checked — a violation surfaces as a *binarystruct.DecodeError.
func TestPacket(t *testing.T) {
	p := Packet{
		Seq:     12345,
		Version: 2,
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	blob, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 4 (Magic) + 4 (Seq) + 1 (Version) + 8 (Payload) = 17 bytes
	if len(blob) != 17 {
		t.Fatalf("expected 17 bytes, got %d", len(blob))
	}

	var decoded Packet
	if err := decoded.UnmarshalBinary(blob); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Magic != [4]byte{'P', 'A', 'K', '1'} {
		t.Errorf("Magic = %q, want PAK1 (the const signature)", decoded.Magic)
	}
	if decoded.Seq != 12345 || decoded.Version != 2 || !bytes.Equal(decoded.Payload, p.Payload) {
		t.Errorf("decoded mismatch: %+v", decoded)
	}

	// An out-of-range Version is rejected with a *DecodeError naming the field.
	bad := append([]byte(nil), blob...)
	bad[8] = 0 // the Version byte (after 4-byte Magic + 4-byte Seq) → out of [1..10]
	var de *binarystruct.DecodeError
	if err := (&Packet{}).UnmarshalBinary(bad); !errors.As(err, &de) || de.Field != "Version" {
		t.Fatalf("expected a DecodeError on Version, got %v", err)
	}
}

// crcMarshaler returns a Marshaler with a "CRC32" valueof evaluator registered.
// The evaluator hashes each referenced field's *encoded bytes* (a.Bytes).
func crcMarshaler() *binarystruct.Marshaler {
	ms := binarystruct.NewMarshaler()
	ms.AddValueOf("CRC32", func(c binarystruct.ValueOfContext) (uint64, error) {
		h := crc32.NewIEEE()
		for _, a := range c.Args {
			h.Write(a.Bytes)
		}
		return uint64(h.Sum32()), nil
	})
	return ms
}

// TestChunk shows the custom-valueof workflow. Because Chunk's CRC field uses a
// runtime-registered evaluator, encode and decode go through the *WithMarshaler
// methods with a Marshaler that has "CRC32" registered. The CRC is computed on
// encode; decode re-checks it (on by default), so a corrupted CRC is rejected.
func TestChunk(t *testing.T) {
	ms := crcMarshaler()

	in := Chunk{Type: "IHDR", Data: []byte{1, 2, 3, 4, 5}}
	var b bytes.Buffer
	if _, err := in.WriteBinaryWithMarshaler(ms, &b, binarystruct.BigEndian); err != nil {
		t.Fatalf("encode: %v", err)
	}
	blob := b.Bytes()
	// 4 (Length) + 4 (Type) + 5 (Data) + 4 (CRC) = 17 bytes
	if len(blob) != 17 {
		t.Fatalf("len = %d, want 17: % x", len(blob), blob)
	}

	// The generated encode filled Length (= len(Data)) and CRC for us.
	wantCRC := crc32.ChecksumIEEE(append([]byte("IHDR"), in.Data...))
	gotCRC := binarystruct.BigEndian.Uint32(blob[len(blob)-4:])
	if gotCRC != wantCRC {
		t.Fatalf("encoded CRC = %#08x, want %#08x", gotCRC, wantCRC)
	}

	var out Chunk
	if _, err := out.ReadBinaryWithMarshaler(ms, bytes.NewReader(blob), binarystruct.BigEndian); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Type != "IHDR" || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("round-trip mismatch: %+v", out)
	}

	// Decode validation is on by default: a corrupted CRC is a *DecodeError.
	corrupt := append([]byte(nil), blob...)
	corrupt[len(corrupt)-1] ^= 0xff
	var de *binarystruct.DecodeError
	if _, err := (&Chunk{}).ReadBinaryWithMarshaler(ms, bytes.NewReader(corrupt), binarystruct.BigEndian); !errors.As(err, &de) || de.Field != "CRC" {
		t.Fatalf("expected a DecodeError on CRC, got %v", err)
	}
}
