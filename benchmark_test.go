// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"testing"

	bst "github.com/mixcode/binarystruct"
)

type benchStruct struct {
	U8  uint8
	U16 uint16
	U32 uint32
	U64 uint64
	I8  int8
	I16 int16
	I32 int32
	I64 int64
	F32 float32
	F64 float64
	S   string `binary:"string(10)"`
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

type sliceBenchStruct struct {
	Data []uint32 `binary:"[1000]uint32"`
}

func BenchmarkMarshalSliceNative(b *testing.B) {
	data := make([]uint32, 1000)
	for i := range data {
		data[i] = uint32(i)
	}
	in := sliceBenchStruct{Data: data}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Marshal(in, bst.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalSliceSwap(b *testing.B) {
	data := make([]uint32, 1000)
	for i := range data {
		data[i] = uint32(i)
	}
	in := sliceBenchStruct{Data: data}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Marshal(in, bst.BigEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSliceNative(b *testing.B) {
	data := make([]uint32, 1000)
	for i := range data {
		data[i] = uint32(i)
	}
	in := sliceBenchStruct{Data: data}
	blob, err := bst.Marshal(in, bst.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	var out sliceBenchStruct
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Unmarshal(blob, bst.LittleEndian, &out)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSliceSwap(b *testing.B) {
	data := make([]uint32, 1000)
	for i := range data {
		data[i] = uint32(i)
	}
	in := sliceBenchStruct{Data: data}
	blob, err := bst.Marshal(in, bst.BigEndian)
	if err != nil {
		b.Fatal(err)
	}
	var out sliceBenchStruct
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Unmarshal(blob, bst.BigEndian, &out)
		if err != nil {
			b.Fatal(err)
		}
	}
}
