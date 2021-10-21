// Copyright 2021 github.com/mixcode

package binarystruct_test

import (
	//"bytes"
	//"fmt"
	//"reflect"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

func benchmarkScalarUnmarshal(b *testing.B, endian bst.ByteOrder) {
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
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}

func BenchmarkScalarUnmarshalLE(b *testing.B) {
	benchmarkScalarUnmarshal(b, bst.LittleEndian)
}
func BenchmarkScalarUnmarshalBE(b *testing.B) {
	benchmarkScalarUnmarshal(b, bst.BigEndian)
}

func benchmarkBitmapUnmarshal(b *testing.B, endian bst.ByteOrder) {
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
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e := bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}

func BenchmarkBitmapUnmarshalLE(b *testing.B) {
	benchmarkBitmapUnmarshal(b, bst.LittleEndian)
}
func BenchmarkBitmapUnmarshalBE(b *testing.B) {
	benchmarkBitmapUnmarshal(b, bst.BigEndian)
}

func benchmarkSliceUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int16
	}
	in := st{[]int16{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceUnmarshalLE(b *testing.B) {
	benchmarkSliceUnmarshal(b, bst.LittleEndian)
}
func BenchmarkSliceUnmarshalBE(b *testing.B) {
	benchmarkSliceUnmarshal(b, bst.BigEndian)
}

func benchmarkSliceConvUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int `binary:"[4]int8"`
	}
	in := st{[]int{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvUnmarshalLE(b *testing.B) {
	benchmarkSliceConvUnmarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvUnmarshalBE(b *testing.B) {
	benchmarkSliceConvUnmarshal(b, bst.BigEndian)
}

func benchmarkSliceConvUnfitUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int `binary:"[8]int8"`
	}
	in := st{[]int{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvUnfitUnmarshalLE(b *testing.B) {
	benchmarkSliceConvUnfitUnmarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvUnfitUnmarshalBE(b *testing.B) {
	benchmarkSliceConvUnfitUnmarshal(b, bst.BigEndian)
}

func benchmarkSliceConvAnyUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A []int8 `binary:"[8]"`
	}
	in := st{[]int8{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkSliceConvAnyUnmarshalLE(b *testing.B) {
	benchmarkSliceConvAnyUnmarshal(b, bst.LittleEndian)
}
func BenchmarkSliceConvAnyUnmarshalBE(b *testing.B) {
	benchmarkSliceConvAnyUnmarshal(b, bst.BigEndian)
}

func benchmarkArrayConvUnfitUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A [4]int `binary:"[]int8"`
	}
	in := st{[4]int{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkArrayConvUnfitUnmarshalLE(b *testing.B) {
	benchmarkArrayConvUnfitUnmarshal(b, bst.LittleEndian)
}
func BenchmarkArrayConvUnfitUnmarshalBE(b *testing.B) {
	benchmarkArrayConvUnfitUnmarshal(b, bst.BigEndian)
}

func benchmarkArrayFitUnfitUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		A [4]int `binary:"[8]int8"`
	}
	in := st{[4]int{1, 2, 3, 4}}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkArrayFitUnfitUnmarshalLE(b *testing.B) {
	benchmarkArrayFitUnfitUnmarshal(b, bst.LittleEndian)
}
func BenchmarkArrayFitUnfitUnmarshalBE(b *testing.B) {
	benchmarkArrayFitUnfitUnmarshal(b, bst.BigEndian)
}

func benchmarkPaddingUnfitUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		PADDING1 interface{} `binary:"pad"`    // single byte
		PADDING2 interface{} `binary:"pad(8)"` // 8 bytes
		PADDING3 interface{} `binary:"[4]pad"` // 4 bytes
	}
	in := st{}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkPaddingUnfitUnmarshalLE(b *testing.B) {
	benchmarkPaddingUnfitUnmarshal(b, bst.LittleEndian)
}
func BenchmarkPaddingUnfitUnmarshalBE(b *testing.B) {
	benchmarkPaddingUnfitUnmarshal(b, bst.BigEndian)
}

func benchmarkStringUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringUnmarshalLE(b *testing.B) {
	benchmarkStringUnmarshal(b, bst.LittleEndian)
}
func BenchmarkStringUnmarshalBE(b *testing.B) {
	benchmarkStringUnmarshal(b, bst.BigEndian)
}

func benchmarkStringVarsizeUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		StringVarsizeLen int16
		Str              string `binary:"string(StringVarsizeLen+1)"`
	}
	s := "hello"
	in := st{int16(len(s)), s}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringVarsizeUnmarshalLE(b *testing.B) {
	benchmarkStringVarsizeUnmarshal(b, bst.LittleEndian)
}
func BenchmarkStringVarsizeUnmarshalBE(b *testing.B) {
	benchmarkStringVarsizeUnmarshal(b, bst.BigEndian)
}

func benchmarkBStringUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"bstring"`
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkBStringUnmarshalLE(b *testing.B) {
	benchmarkBStringUnmarshal(b, bst.LittleEndian)
}
func BenchmarkBStringUnmarshalBE(b *testing.B) {
	benchmarkBStringUnmarshal(b, bst.BigEndian)
}

func benchmarkDWStringUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"dwstring"`
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkDWStringUnmarshalLE(b *testing.B) {
	benchmarkDWStringUnmarshal(b, bst.LittleEndian)
}
func BenchmarkDWStringUnmarshalBE(b *testing.B) {
	benchmarkDWStringUnmarshal(b, bst.BigEndian)
}

func benchmarkStringArrayUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"[3]string(0x10)"` // S matches to string[0]
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringArrayUnmarshalLE(b *testing.B) {
	benchmarkStringArrayUnmarshal(b, bst.LittleEndian)
}
func BenchmarkStringArrayUnmarshalBE(b *testing.B) {
	benchmarkStringArrayUnmarshal(b, bst.BigEndian)
}

func benchmarkStringToByteUnmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"[8]byte"`
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringToByteUnmarshalLE(b *testing.B) {
	benchmarkStringToByteUnmarshal(b, bst.LittleEndian)
}
func BenchmarkStringToByteUnmarshalBE(b *testing.B) {
	benchmarkStringToByteUnmarshal(b, bst.BigEndian)
}

func benchmarkStringToInt16Unmarshal(b *testing.B, endian bst.ByteOrder) {
	type st struct {
		S string `binary:"[8]int16"`
	}
	in := st{"hello"}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStringToInt16UnmarshalLE(b *testing.B) {
	benchmarkStringToInt16Unmarshal(b, bst.LittleEndian)
}
func BenchmarkStringToInt16UnmarshalBE(b *testing.B) {
	benchmarkStringToInt16Unmarshal(b, bst.BigEndian)
}

func benchmarkPointerUnmarshal(b *testing.B, endian bst.ByteOrder) {
	i6 := int32(6)
	p6 := &i6
	type st struct {
		P1 *int32
		P2 **int32
	}
	in := st{p6, &p6}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkPointerUnmarshalLE(b *testing.B) {
	benchmarkPointerUnmarshal(b, bst.LittleEndian)
}
func BenchmarkPointerUnmarshalBE(b *testing.B) {
	benchmarkPointerUnmarshal(b, bst.BigEndian)
}

func benchmarkInterfaceUnmarshal(b *testing.B, endian bst.ByteOrder) {
	i6 := int32(6)
	p6 := &i6
	type st struct {
		I1 interface{}
		I2 interface{}
	}
	in := st{&i6, &p6}
	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		n1, n2 := int32(0), int32(0)
		p2 := &n2
		tmp := st{&n1, &p2} // Interface must be pre-set to be unmarshaled
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkInterfaceUnmarshalLE(b *testing.B) {
	benchmarkInterfaceUnmarshal(b, bst.LittleEndian)
}
func BenchmarkInterfaceUnmarshalBE(b *testing.B) {
	benchmarkInterfaceUnmarshal(b, bst.BigEndian)
}

func benchmarkStructUnmarshal(b *testing.B, endian bst.ByteOrder) {
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

	out, e := bst.Marshal(in, endian)
	if e != nil {
		b.Error(e)
		return
	}
	for i := 0; i < b.N; i++ {
		var tmp st
		_, e = bst.Unmarshal(out, endian, &tmp)
		if e != nil {
			b.Error(e)
			return
		}
	}
}
func BenchmarkStructUnmarshalLE(b *testing.B) {
	benchmarkStructUnmarshal(b, bst.LittleEndian)
}
func BenchmarkStructUnmarshalBE(b *testing.B) {
	benchmarkStructUnmarshal(b, bst.BigEndian)
}
