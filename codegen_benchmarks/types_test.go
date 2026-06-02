// Copyright 2026 github.com/mixcode

package codegen_benchmarks

import (
	"testing"

	bst "github.com/mixcode/binarystruct"
)

func BenchmarkUnmarshal_Comparison(b *testing.B) {
	sub := BenchSubHeader{Type: 1, Sequence: 100, Checksum: 3.14}
	p := BenchPacket{
		Magic:     0xABCD,
		Version:   2,
		Flags:     0x80,
		PayloadSz: 256,
		Id:        "abcdefgh",
		Buffer:    make([]byte, 256),
		Nested:    sub,
	}
	blob, err := bst.Marshal(&p, bst.BigEndian)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("Codegen_Static", func(b *testing.B) {
		var out BenchPacket
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := out.UnmarshalBinary(blob)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Dynamic_Interpreter_Unsafe", func(b *testing.B) {
		var out BenchPacketDynamic
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := bst.Unmarshal(blob, bst.BigEndian, &out)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestUnmarshal_Comparison(t *testing.T) {
	sub := BenchSubHeader{Type: 1, Sequence: 100, Checksum: 3.14}
	p := BenchPacket{
		Magic:     0xABCD,
		Version:   2,
		Flags:     0x80,
		PayloadSz: 256,
		Id:        "abcdefgh",
		Buffer:    make([]byte, 256),
		Nested:    sub,
	}
	blob, err := bst.Marshal(&p, bst.BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	var out BenchPacket
	err = out.UnmarshalBinary(blob)
	if err != nil {
		t.Fatal(err)
	}
	if out.Magic != 0xABCD || out.Id != "abcdefgh" || out.Nested.Sequence != 100 {
		t.Errorf("unmarshalled mismatch: %+v", out)
	}
}

