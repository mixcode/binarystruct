// Copyright 2021 github.com/mixcode

package binarystruct_test

import (
	//"bytes"
	//"fmt"
	//"reflect"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

func benchmarkScalarMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
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
	}
	in := st{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}

func BenchmarkScalarMarshalLE(b *testing.B) {
	benchmarkScalarMarshal(b, bst.LittleEndian)
}
func BenchmarkScalarMarshalBE(b *testing.B) {
	benchmarkScalarMarshal(b, bst.BigEndian)
}

func benchmarkBitmapMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		U8   uint8   `binary:"byte"`
		U16  uint16  `binary:"word"`
		U32  uint32  `binary:"dword"`
		U64  uint64  `binary:"qword"`
		I8   int8    `binary:"byte"`
		I16  int16   `binary:"word"`
		I32  int32   `binary:"dword"`
		I64  int64   `binary:"qword"`
		IB8  int     `binary:"byte"`
		IB16 int     `binary:"word"`
		IB32 int     `binary:"dword"`
		IB64 int     `binary:"qword"`
		F32  float32 `binary:"dword"`
		F64  float64 `binary:"qword"`
	}
	in := st{1, 2, 3, 4, -1, -2, -3, -4, -5, -6, -7, -8, 100., 200.}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}

func BenchmarkBitmapMarshalLE(b *testing.B) {
	benchmarkBitmapMarshal(b, bst.LittleEndian)
}
func BenchmarkBitmapMarshalBE(b *testing.B) {
	benchmarkBitmapMarshal(b, bst.BigEndian)
}

func benchmarkSliceMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int16
	}
	in := st{[]int16{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceMarshalLE(b *testing.B) {
	benchmarkSliceMarshal(b, bst.LittleEndian)
}
func BenchmarkSliceMarshalBE(b *testing.B) {
	benchmarkSliceMarshal(b, bst.BigEndian)
}

func benchmarkSliceConvMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int `binary:"[4]int8"`
	}
	in := st{[]int{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvMarshalLE(b *testing.B) {
	benchmarkSliceConvMarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvMarshalBE(b *testing.B) {
	benchmarkSliceConvMarshal(b, bst.BigEndian)
}

func benchmarkSliceConvUnfitMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int `binary:"[8]int8"`
	}
	in := st{[]int{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvUnfitMarshalLE(b *testing.B) {
	benchmarkSliceConvUnfitMarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvUnfitMarshalBE(b *testing.B) {
	benchmarkSliceConvUnfitMarshal(b, bst.BigEndian)
}

func benchmarkSliceConvAnyMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int8 `binary:"[8]"`
	}
	in := st{[]int8{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvAnyMarshalLE(b *testing.B) {
	benchmarkSliceConvAnyMarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvAnyMarshalBE(b *testing.B) {
	benchmarkSliceConvAnyMarshal(b, bst.BigEndian)
}

func benchmarkArrayConvUnfitMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A [4]int `binary:"[]int8"`
	}
	in := st{[4]int{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkArrayConvUnfitMarshalLE(b *testing.B) {
	benchmarkArrayConvUnfitMarshal(b, bst.LittleEndian)
}
func BenchmarkArrayConvUnfitMarshalBE(b *testing.B) {
	benchmarkArrayConvUnfitMarshal(b, bst.BigEndian)
}

func benchmarkArrayFitUnfitMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A [4]int `binary:"[8]int8"`
	}
	in := st{[4]int{1, 2, 3, 4}}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkArrayFitUnfitMarshalLE(b *testing.B) {
	benchmarkArrayFitUnfitMarshal(b, bst.LittleEndian)
}
func BenchmarkArrayFitUnfitMarshalBE(b *testing.B) {
	benchmarkArrayFitUnfitMarshal(b, bst.BigEndian)
}

func benchmarkPaddingUnfitMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		PADDING1 interface{} `binary:"pad"`    // single byte
		PADDING2 interface{} `binary:"pad(8)"` // 8 bytes
		PADDING3 interface{} `binary:"[4]pad"` // 4 bytes
	}
	in := st{}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkPaddingUnfitMarshalLE(b *testing.B) {
	benchmarkPaddingUnfitMarshal(b, bst.LittleEndian)
}
func BenchmarkPaddingUnfitMarshalBE(b *testing.B) {
	benchmarkPaddingUnfitMarshal(b, bst.BigEndian)
}

func benchmarkStringMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string
	}
	in := st{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringMarshalLE(b *testing.B) {
	benchmarkStringMarshal(b, bst.LittleEndian)
}
func BenchmarkStringMarshalBE(b *testing.B) {
	benchmarkStringMarshal(b, bst.BigEndian)
}

func benchmarkStringVarsizeMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		StringVarsizeLen int16
		Str              string `binary:"string(StringVarsizeLen+1)"`
	}
	s := "hello"
	in := st{int16(len(s)), s}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringVarsizeMarshalLE(b *testing.B) {
	benchmarkStringVarsizeMarshal(b, bst.LittleEndian)
}
func BenchmarkStringVarsizeMarshalBE(b *testing.B) {
	benchmarkStringVarsizeMarshal(b, bst.BigEndian)
}

func benchmarkBStringMarshal(b *testing.B, endian bst.ByteOrder) {
	type st1 struct {
		S string `binary:"bstring"`
	}
	in := st1{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkBStringMarshalLE(b *testing.B) {
	benchmarkBStringMarshal(b, bst.LittleEndian)
}
func BenchmarkBStringMarshalBE(b *testing.B) {
	benchmarkBStringMarshal(b, bst.BigEndian)
}

func benchmarkDWStringMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"dwstring"`
	}
	in := st{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkDWStringMarshalLE(b *testing.B) {
	benchmarkDWStringMarshal(b, bst.LittleEndian)
}
func BenchmarkDWStringMarshalBE(b *testing.B) {
	benchmarkDWStringMarshal(b, bst.BigEndian)
}

func benchmarkStringArrayMarshal(b *testing.B, endian bst.ByteOrder) {
	type st1 struct {
		S string `binary:"[3]string(0x10)"` // S matches to string[0]
	}
	in := st1{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringArrayMarshalLE(b *testing.B) {
	benchmarkStringArrayMarshal(b, bst.LittleEndian)
}
func BenchmarkStringArrayMarshalBE(b *testing.B) {
	benchmarkStringArrayMarshal(b, bst.BigEndian)
}

func benchmarkStringToByteMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"[8]byte"`
	}
	in := st{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringToByteMarshalLE(b *testing.B) {
	benchmarkStringToByteMarshal(b, bst.LittleEndian)
}
func BenchmarkStringToByteMarshalBE(b *testing.B) {
	benchmarkStringToByteMarshal(b, bst.BigEndian)
}

func benchmarkStringToInt16Marshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"[8]int16"`
	}
	in := st{"hello"}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringToInt16MarshalLE(b *testing.B) {
	benchmarkStringToInt16Marshal(b, bst.LittleEndian)
}
func BenchmarkStringToInt16MarshalBE(b *testing.B) {
	benchmarkStringToInt16Marshal(b, bst.BigEndian)
}

func benchmarkPointerMarshal(b *testing.B, endian bst.ByteOrder) {
	i6 := int32(6)
	p6 := &i6
	type st struct {
		P1 *int32
		P2 **int32
	}
	in := st{p6, &p6}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkPointerMarshalLE(b *testing.B) {
	benchmarkPointerMarshal(b, bst.LittleEndian)
}
func BenchmarkPointerMarshalBE(b *testing.B) {
	benchmarkPointerMarshal(b, bst.BigEndian)
}

func benchmarkInterfaceMarshal(b *testing.B, endian bst.ByteOrder) {
	i6 := int32(6)
	p6 := &i6
	type st struct {
		I1 interface{}
		I2 interface{}
	}
	in := st{&i6, &p6}
	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkInterfaceMarshalLE(b *testing.B) {
	benchmarkInterfaceMarshal(b, bst.LittleEndian)
}
func BenchmarkInterfaceMarshalBE(b *testing.B) {
	benchmarkInterfaceMarshal(b, bst.BigEndian)
}

func benchmarkStructMarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		U1     int    `binary:"uint8"`
		I2     int    `binary:"word"`
		B3     bool   `binary:"int8"`
		IGNORE int    `binary:"ignore"` // ignore this value
		N4     string `binary:"wstring"`
		//N5 string `binary:"string(10),encoding=utf16"`	// string encoding not implemented yet
		N5 string `binary:"string(10)"`
		A6 []int  `binary:"[8]byte"`
		P1 *int16
		P2 **int16
		S7 struct {
			F1 float32
			I2 int32
		}
		unexported int
	}

	i6 := int16(6)
	p6 := &i6
	in := st{
		1,
		2,
		true,
		9999, // ignoring value
		"hello",
		"hello2",
		[]int{1, 2, 3, 4, 5},
		p6, &p6,
		struct {
			F1 float32
			I2 int32
		}{12.34, 0x01020304},
		999, // unexported value
	}

	for i := 0; i < b.N; i++ {
		_, e := bst.Marshal(in, endian)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStructMarshalLE(b *testing.B) {
	benchmarkStructMarshal(b, bst.LittleEndian)
}
func BenchmarkStructMarshalBE(b *testing.B) {
	benchmarkStructMarshal(b, bst.BigEndian)
}
