// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"testing"

	bst "github.com/mixcode/binarystruct"
)

type benchStruct struct {
	U8   uint8
	U16  uint16
	U32  uint32
	U64  uint64
	I8   int8
	I16  int16
	I32  int32
	I64  int64
	F32  float32
	F64  float64
	S    string `binary:"string(10)"`
}

func BenchmarkMarshal(b *testing.B) {
	in := benchStruct{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1, "hello"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Marshal(in, bst.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	in := benchStruct{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1, "hello"}
	blob, err := bst.Marshal(in, bst.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	var out benchStruct
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Unmarshal(blob, bst.LittleEndian, &out)
		if err != nil {
			b.Fatal(err)
		}
	}
}
