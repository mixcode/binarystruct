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

type benchValidationRangeStruct struct {
	U8  uint8   `binary:"uint8,range=0..10"`
	U16 uint16  `binary:"uint16,range=0..10"`
	U32 uint32  `binary:"uint32,range=0..10"`
	U64 uint64  `binary:"uint64,range=0..10"`
	I8  int8    `binary:"int8,range=-10..10"`
	I16 int16   `binary:"int16,range=-10..10"`
	I32 int32   `binary:"int32,range=-10..10"`
	I64 int64   `binary:"int64,range=-10..10"`
	F32 float32 `binary:"float32,range=0..10"`
	F64 float64 `binary:"float64,range=0..10"`
	S   string  `binary:"string(10)"`
}

type benchValidationRegexStruct struct {
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
	S   string `binary:"string(10),match=^[a-z]+$"`
}

func BenchmarkUnmarshal_RangeValidation(b *testing.B) {
	in := benchValidationRangeStruct{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1, "hello"}
	blob, err := bst.Marshal(in, bst.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	var out benchValidationRangeStruct
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Unmarshal(blob, bst.LittleEndian, &out)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_RegexValidation(b *testing.B) {
	in := benchValidationRegexStruct{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1, "hello"}
	blob, err := bst.Marshal(in, bst.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	var out benchValidationRegexStruct
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bst.Unmarshal(blob, bst.LittleEndian, &out)
		if err != nil {
			b.Fatal(err)
		}
	}
}
