// Copyright 2021 mixcode@github

package binarystruct_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/mixcode/binarystruct"
	bst "github.com/mixcode/binarystruct"
)

func printHex(b []byte) {
	sz := 8
	i := 0
	for i < len(b) {
		fmt.Printf("0x%02x, ", b[i])
		i++
		if i%sz == 0 {
			fmt.Println()
		}
	}
	if i%sz != 0 {
		fmt.Println()
	}
}

func TestStruct(test *testing.T) {

	var res []byte

	// functions to compare encoded data
	encodeCompare := func(data interface{}, desired []byte, endian bst.ByteOrder) {
		res = nil
		var e error
		res, e = bst.Marshal(data, endian)
		if e != nil {
			test.Error(e)
			return
		}
		if !bytes.Equal(res, desired) {
			test.Errorf("written data not matching")
		}
	}
	encodeCompareLE := func(s interface{}, desired []byte) { // compare with LittleEndian results
		encodeCompare(s, desired, bst.LittleEndian)
	}
	encodeCompareBE := func(s interface{}, desired []byte) { // compare with BigEndian results
		encodeCompare(s, desired, bst.BigEndian)
	}
	_, _ = encodeCompareLE, encodeCompareBE

	// dfunctions to compare decoded data
	decodeCompare := func(data []byte, out interface{}, endian bst.ByteOrder, original interface{}) {
		n, err := bst.Unmarshal(data, endian, out)
		if err != nil {
			test.Error(err)
			return
		}
		if n != len(data) {
			test.Errorf("invalid read size")
			return
		}

		if !reflect.DeepEqual(original, out) {
			test.Errorf("decoded data is not equal to original")
		}
	}
	decodeCompareLE := func(data []byte, out interface{}, compare interface{}) { // compare with LittleEndian results
		decodeCompare(data, out, bst.LittleEndian, compare)
	}
	decodeCompareBE := func(data []byte, out interface{}, compare interface{}) { // compare with BigEndian results
		decodeCompare(data, out, bst.BigEndian, compare)
	}
	_, _ = decodeCompareLE, decodeCompareBE

	// scalar values
	func() {
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
		expLE := []byte{ // in little endian
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x66, 0x66,
			0x66, 0x3f, 0x9a, 0x99, 0x99, 0x99, 0x99, 0x99,
			0xf1, 0x3f,
		}
		expBE := []byte{ // in big endian
			0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0xff,
			0xff, 0xfe, 0xff, 0xff, 0xff, 0xfd, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xfc, 0x3f, 0x66,
			0x66, 0x66, 0x3f, 0xf1, 0x99, 0x99, 0x99, 0x99,
			0x99, 0x9a,
		}

		encodeCompareLE(in, expLE)
		//printHex(exp)
		out := st{}
		decodeCompareLE(expLE, &out, &in)

		encodeCompareBE(in, expBE)
		outBE := st{}
		decodeCompareBE(expBE, &outBE, &in)
	}()

	// bitmap image types
	func() {
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
		expLE := []byte{ // in little endian
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfb, 0xfa,
			0xff, 0xf9, 0xff, 0xff, 0xff, 0xf8, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0xc8,
			0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x69,
			0x40,
		}
		expBE := []byte{ // in big endian
			0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0xff,
			0xff, 0xfe, 0xff, 0xff, 0xff, 0xfd, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xfc, 0xfb, 0xff,
			0xfa, 0xff, 0xff, 0xff, 0xf9, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xf8, 0x42, 0xc8, 0x00,
			0x00, 0x40, 0x69, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00,
		}
		encodeCompareLE(in, expLE)
		correct := st{1, 2, 3, 4, -1, -2, -3, -4,
			// Note: sign-agnostic types could be ambiguous and may not be recovered to original when mapped to a larger type
			251, 65530, 4294967289, -8,
			100., 200.}
		out := st{}
		decodeCompareLE(expLE, &out, &correct)

		encodeCompareBE(in, expBE)
		out2 := st{}
		decodeCompareBE(expBE, &out2, &correct)
	}()

	func() {
		type st struct {
			A []int16
		}
		in := st{[]int16{1, 2, 3, 4}}
		exp := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}
		encodeCompareLE(in, exp)
		out := st{A: make([]int16, 4)} // the slice size will be the array size to be read
		decodeCompareLE(exp, &out, &in)
	}()

	// slice with type conversion and just-fit size
	func() {
		type st struct {
			A []int `binary:"[4]int8"`
		}
		in := st{[]int{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04}
		encodeCompareLE(in, exp)
		out := st{} // the slice is automatically allocated if the slice is nil and the length is explicitly given
		decodeCompareLE(exp, &out, &in)
	}()

	// slice with type conversion and fixed size
	func() {
		type st struct {
			A []int `binary:"[8]int8"`
		}
		in := st{[]int{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		encodeCompareLE(in, exp)
		in.A = append(in.A, []int{0, 0, 0, 0}...) // decoded slice will be 8 elements long
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// slice with explicit 'any' type
	func() {
		type st struct {
			A []int8 `binary:"[8]any"`
		}
		in := st{[]int8{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		encodeCompareLE(in, exp)
		in.A = append(in.A, []int8{0, 0, 0, 0}...) // decoded slice will be 8 elements long
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// slice with implicit 'any' type
	func() {
		type st struct {
			A []int8 `binary:"[8]"`
		}
		in := st{[]int8{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		encodeCompareLE(in, exp)
		in.A = append(in.A, []int8{0, 0, 0, 0}...) // decoded slice will be 8 elements long
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// array with type conversion
	func() {
		type st struct {
			A [4]int `binary:"[]int8"`
		}
		in := st{[4]int{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04}
		encodeCompareLE(in, exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// array with type conversion and fixed size
	func() {
		type st struct {
			A [4]int `binary:"[8]int8"`
		}
		in := st{[4]int{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		encodeCompareLE(in, exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// zero padding bytes
	func() {
		type st struct {
			PADDING1 interface{} `binary:"pad"`    // single byte
			PADDING2 interface{} `binary:"pad(8)"` // 8 bytes
			PADDING3 interface{} `binary:"[4]pad"` // 4 bytes
		}
		in := st{}
		exp := []byte{
			0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0,
		}
		encodeCompareLE(in, exp)
		out := st{}
		// Note: zero type will read no data; just skips bytes.
		decodeCompareLE(exp, &out, &in)
	}()

	// string
	func() {
		type st struct {
			S string
		}
		in := st{"hello"}
		exp := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)
		out := struct {
			S string `binary:"string(5)"`
		}{}
		_, e := bst.Unmarshal(exp, bst.LittleEndian, &out)
		if e != nil {
			test.Error(e)
			return
		}
		if in.S != out.S {
			test.Errorf("decode failed: string mismatch")
		}
	}()

	// string with size reference
	func() {
		type st struct {
			StringLen int16
			Str       string `binary:"string(StringLen+1)"`
		}
		s := "hello"
		in := st{int16(len(s)), s}
		exp := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		encodeCompareLE(in, exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// string with multiple size reference including an unexported field
	// TODO: do more tests on unexported fields??
	func() {
		type st struct {
			N         uint8
			stringLen int16  // this is an unexported field
			Str       string `binary:"string(stringLen +2 -N)"`
		}
		s := "hello"
		in := st{1, int16(len(s)), s}
		exp := []byte{0x01, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		encodeCompareLE(in, exp)
		out := st{stringLen: int16(len(s))} // value of unexported fields must be given
		decodeCompareLE(exp, &out, &in)
	}()

	// bstring
	func() {
		type st1 struct {
			S string `binary:"bstring"`
		}
		in := st1{"hello"}
		exp := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)
		out := st1{}
		decodeCompareLE(exp, &out, &in)

		// fixed buffer size bstring
		type st2 struct {
			S string `binary:"bstring(10)"`
		}
		in2 := st2{"hello"}
		exp2 := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
		out2 := st2{}
		decodeCompareLE(exp2, &out2, &in2)
	}()

	// wstring
	func() {
		type st1 struct {
			S string `binary:"wstring"`
		}
		in := st1{"hello"}
		exp := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)
		out := st1{}
		decodeCompareLE(exp, &out, &in)

		// fixed buffer size wstring
		type st2 struct {
			S string `binary:"wstring(10)"`
		}
		in2 := st2{"hello"}
		exp2 := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
		out2 := st2{}
		decodeCompareLE(exp2, &out2, &in2)
	}()

	// dwstring
	func() {
		type st1 struct {
			S string `binary:"dwstring"`
		}
		in := st1{"hello"}
		exp := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)
		out := st1{}
		decodeCompareLE(exp, &out, &in)

		// fixed buffer size dwstring
		type st2 struct {
			S string `binary:"dwstring(10)"`
		}
		in2 := st2{"hello"}
		exp2 := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
		out2 := st2{}
		decodeCompareLE(exp2, &out2, &in2)
	}()

	// string to []string
	func() {
		type st1 struct {
			S string `binary:"[3]string(0x10)"` // S matches to string[0]
		}
		in := st1{"hello"}
		exp := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		encodeCompareLE(in, exp)
		out := st1{}
		decodeCompareLE(exp, &out, &in)

		type st2 struct {
			S string `binary:"[3]string(5)"`
		}
		in2 := st2{"hello"}
		exp2 := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f,
			0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00,
		}
		encodeCompareLE(in2, exp2)
		out2 := st2{}
		decodeCompareLE(exp2, &out2, &in2)
	}()

	// string to []byte
	func() {
		type st struct {
			S string `binary:"[8]byte"`
		}
		in := st{"hello"}
		exp := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00}
		encodeCompareLE(in, exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()
	//printHex(res)

	// string to []int16
	func() {
		type st struct {
			S string `binary:"[8]int16"`
		}
		in := st{"hello"}
		exp := []byte{0x68, 0, 0x65, 0, 0x6c, 0, 0x6c, 0, 0x6f, 0, 0x00, 0, 0x00, 0, 0x00, 0}
		encodeCompareLE(in, exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
	}()

	// pointer deference
	func() {
		i6 := int32(6)
		p6 := &i6
		type st struct {
			P1 *int32
			P2 **int32
		}
		in := st{p6, &p6}
		exp := []byte{0x06, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00}
		encodeCompareLE(in, exp)
		out := st{} // nil pointers are automatically allocated
		decodeCompareLE(exp, &out, &in)

		var c1, c2 int32
		p2 := &c2
		out2 := st{&c1, &p2} // non-nil pointers are used as-is
		b1, b2 := out2.P1, out2.P2
		decodeCompareLE(exp, &out2, &in)
		if b1 != out2.P1 || b2 != out2.P2 {
			test.Errorf("pointer changed")
		}
	}()

	// interface
	func() {
		i6 := int32(6)
		p6 := &i6
		type st struct {
			I1 interface{}
			I2 interface{}
		}
		in := st{&i6, &p6}
		exp := []byte{0x06, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00}
		encodeCompareLE(in, exp)

		n1, n2 := int32(0), int32(0)
		p2 := &n2
		out := st{&n1, &p2} // Interface must be pre-set to be unmarshaled
		decodeCompareLE(exp, &out, &in)
	}()

	// some complex structure
	func() {
		type t struct {
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
		in := t{
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
		exp := []byte{
			0x01, 0x02, 0x00, 0x01, 0x05, 0x00, 0x68, 0x65,
			0x6c, 0x6c, 0x6f, 0x68, 0x65, 0x6c, 0x6c, 0x6f,
			0x32, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03,
			0x04, 0x05, 0x00, 0x00, 0x00, 0x06, 0x00, 0x06,
			0x00, 0xa4, 0x70, 0x45, 0x41, 0x04, 0x03, 0x02,
			0x01,
		}
		encodeCompareLE(in, exp)

		// set implicitly written values
		in.A6 = append(in.A6, []int{0, 0, 0}...)
		// clear ignored values
		in.IGNORE, in.unexported = 0, 0
		out := t{}
		decodeCompareLE(exp, &out, &in)
	}()

}

func ExampleMarshal() {
	strc := struct {
		Header       string `binary:"[4]byte"` // marshaled to 4 bytes
		ValueInt8    int    `binary:"int8"`    // marshaled to single byte
		ValueUint16  int    `binary:"uint16"`  // marshaled to two bytes
		ValueDword32 int    `binary:"dword"`   // marshaled to four bytes
	}{"abcd", 1, 2, 3}
	blob, err := binarystruct.Marshal(strc, binarystruct.BigEndian)

	if err != nil {
		panic(err)
	}
	for _, b := range blob {
		fmt.Printf(" %02x", b)
	}
	fmt.Println()

	// Output:
	// 61 62 63 64 01 00 02 00 00 00 03
}

func ExampleUnmarshal() {
	blob := []byte{0x61, 0x62, 0x63, 0x64,
		0x01,
		0x00, 0x02,
		0x00, 0x00, 0x00, 0x03}
	// [ "abcd", 0x01, 0x0002, 0x00000003 ]

	// A quick example
	strc := struct {
		Header       string `binary:"[4]byte"` // marshaled to 4 bytes
		ValueInt8    int    `binary:"int8"`    // marshaled to single byte
		ValueUint16  int    `binary:"uint16"`  // marshaled to two bytes
		ValueDword32 int    `binary:"dword"`   // marshaled to four bytes
	}{}
	readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

	if err != nil {
		panic(err)
	}
	fmt.Println(readsz, strc)

	// Output:
	// 11 {abcd 1 2 3}
}
