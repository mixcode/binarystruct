package binarystruct_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

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
	decodeCompare := func(data []byte, out interface{}, endian bst.ByteOrder, compare interface{}) {
		n, err := bst.Unmarshal(data, endian, out)
		if err != nil {
			test.Error(err)
			return
		}
		if n != len(data) {
			test.Errorf("invalid read size")
			return
		}

		if !reflect.DeepEqual(compare, out) {
			test.Errorf("decoded data is not equal")
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
		exp := []byte{
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x66, 0x66,
			0x66, 0x3f, 0x9a, 0x99, 0x99, 0x99, 0x99, 0x99,
			0xf1, 0x3f,
		}
		encodeCompareLE(in, exp)
		//printHex(exp)
		out := st{}
		decodeCompareLE(exp, &out, &in)
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
		exp := []byte{
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfb, 0xfa,
			0xff, 0xf9, 0xff, 0xff, 0xff, 0xf8, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0xc8,
			0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x69,
			0x40,
		}
		encodeCompareLE(in, exp)
		correct := st{1, 2, 3, 4, -1, -2, -3, -4,
			// Note that sign-agnostic types could be ambiguous and may not be recovered to original when mapped to a larger type
			251, 65530, 4294967289, -8,
			100., 200.}
		out := st{}
		decodeCompareLE(exp, &out, &correct)
	}()

	func() {
		type st struct {
			A []int16
		}
		in := st{[]int16{1, 2, 3, 4}}
		exp := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}
		encodeCompareLE(in, exp)
		out := struct {
			A []int16 `binary:"[4]"` // exact size must be given for decoding
		}{}
		_, err := bst.Unmarshal(exp, bst.LittleEndian, &out)
		if err != nil {
			test.Error(err)
			return
		}
		// note: in and out is not a same type and cannot be compared
		decodeCompareLE(exp, &out.A, &in.A)
	}()

	// slice with type conversion and just-fit size
	func() {
		type st struct {
			A []int `binary:"[4]int8"`
		}
		in := st{[]int{1, 2, 3, 4}}
		exp := []byte{0x01, 0x02, 0x03, 0x04}
		encodeCompareLE(in, exp)
		out := st{}
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

	// zero bytes
	func() {
		type st struct {
			X interface{} `binary:"zero"`
			Y interface{} `binary:"zero(8)"`
			Z interface{} `binary:"[4]zero"`
		}
		in := st{}
		exp := []byte{
			0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0,
		}
		encodeCompareLE(in, exp)
		out := st{}
		// zero type will read no data; just skip bytes
		sz, e := bst.Unmarshal(exp, bst.LittleEndian, &out)
		if e != nil {
			test.Error(e)
		}
		if sz != len(exp) {
			test.Errorf("read size not match")
		}
	}()
	return

	// string
	func() {
		type t struct {
			S string
		}
		in := t{"hello"}
		exp := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)
	}()

	// string with size reference
	func() {
		type t struct {
			StringLen int16
			Str       string `binary:"string(StringLen+1)"`
		}
		s := "hello"
		in := t{int16(len(s)), s}
		exp := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		encodeCompareLE(in, exp)
	}()

	// string with multiple size reference including an unexported field
	// note: this may nt
	func() {
		type t struct {
			N         uint8
			stringLen int16  // this is an unexported field
			Str       string `binary:"string(stringLen +2 -N)"`
		}
		s := "hello"
		in := t{1, int16(len(s)), s}
		exp := []byte{0x01, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		encodeCompareLE(in, exp)
	}()

	// bstring
	func() {
		type t struct {
			S string `binary:"bstring"`
		}
		in := t{"hello"}
		exp := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)

		// fixed buffer size bstring
		type t2 struct {
			S string `binary:"bstring(10)"`
		}
		in2 := t2{"hello"}
		exp2 := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
	}()

	// wstring
	func() {
		type t struct {
			S string `binary:"wstring"`
		}
		in := t{"hello"}
		exp := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)

		// fixed buffer size wstring
		type t2 struct {
			S string `binary:"wstring(10)"`
		}
		in2 := t2{"hello"}
		exp2 := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
	}()

	// dwstring
	func() {
		type t struct {
			S string `binary:"dwstring"`
		}
		in := t{"hello"}
		exp := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		encodeCompareLE(in, exp)

		// fixed buffer size dwstring
		type t2 struct {
			S string `binary:"dwstring(10)"`
		}
		in2 := t2{"hello"}
		exp2 := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		encodeCompareLE(in2, exp2)
	}()

	// string to []string
	func() {
		type t struct {
			S string `binary:"[3]string(0x10)"` // S matches to string[0]
		}
		in := t{"hello"}
		exp := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		encodeCompareLE(in, exp)

		type t2 struct {
			S string `binary:"[3]string(5)"`
		}
		in2 := t2{"hello"}
		exp2 := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f,
			0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00,
		}
		encodeCompareLE(in2, exp2)
	}()

	// string to []byte
	func() {
		type t struct {
			S string `binary:"[8]byte"`
		}
		in := t{"hello"}
		exp := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00}
		encodeCompareLE(in, exp)
	}()
	//printHex(res)

	// string to []int16
	func() {
		type t struct {
			S string `binary:"[8]int16"`
		}
		in := t{"hello"}
		exp := []byte{0x68, 0, 0x65, 0, 0x6c, 0, 0x6c, 0, 0x6f, 0, 0x00, 0, 0x00, 0, 0x00, 0}
		encodeCompareLE(in, exp)
	}()

	// pointer deference
	func() {
		i6 := int32(6)
		p6 := &i6
		type tp struct {
			P1 *int32
			P2 **int32
		}
		sp := tp{p6, &p6}
		op := []byte{0x06, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00}
		encodeCompareLE(sp, op)
	}()

	// interface
	func() {
		i6 := int32(6)
		type ti struct {
			I1 interface{}
			I2 interface{}
		}
		si := ti{i6, &i6}
		oi := []byte{0x06, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00}
		encodeCompareLE(si, oi)
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
			999,
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
	}()

}
