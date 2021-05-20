package binarystruct_test

import (
	"bytes"
	"fmt"
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

func TestStruct(t *testing.T) {

	var res []byte
	compare := func(s interface{}, desired []byte, endian bst.ByteOrder) { // compare with LittleEndian results
		res = nil
		var buf bytes.Buffer
		sz, err := bst.Write(&buf, endian, s)
		if err != nil {
			t.Error(err)
			return
		}
		if sz != len(buf.Bytes()) {
			t.Errorf("invalid write size")
			return
		}
		res = buf.Bytes()
		if !bytes.Equal(res, desired) {
			t.Errorf("written data not matching")
		}
	}

	compareLE := func(s interface{}, desired []byte) { // compare with LittleEndian results
		compare(s, desired, bst.LittleEndian)
	}
	compareBE := func(s interface{}, desired []byte) { // compare with LittleEndian results
		compare(s, desired, bst.BigEndian)
	}
	_, _ = compareLE, compareBE

	// scalar values
	func() {
		type t struct {
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
		in := t{1, 2, 3, 4, -1, -2, -3, -4, 0.9, 1.1}
		out := []byte{
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x66, 0x66,
			0x66, 0x3f, 0x9a, 0x99, 0x99, 0x99, 0x99, 0x99,
			0xf1, 0x3f,
		}
		compareLE(in, out)
	}()

	// scalars with type conversion
	func() {
		type t struct {
			U8  int `binary:"uint8"`
			U16 int `binary:"uint16"`
			U32 int `binary:"uint32"`
			U64 int `binary:"uint64"`
			I8  int `binary:"int8"`
			I16 int `binary:"int16"`
			I32 int `binary:"int32"`
			I64 int `binary:"int64"`
			F32 int `binary:"float32"`
			F64 int `binary:"float64"`
		}
		in := t{1, 2, 3, 4, -1, -2, -3, -4, 100, 200}
		out := []byte{
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00,
			0xc8, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x69, 0x40,
		}
		compareLE(in, out)
	}()

	// bitmap image types
	func() {
		type t struct {
			U8  uint    `binary:"byte"`
			U16 uint    `binary:"word"`
			U32 uint    `binary:"dword"`
			U64 uint    `binary:"qword"`
			I8  int     `binary:"byte"`
			I16 int     `binary:"word"`
			I32 int     `binary:"dword"`
			I64 int     `binary:"qword"`
			F32 float32 `binary:"dword"`
			F64 float64 `binary:"qword"`
		}
		in := t{1, 2, 3, 4, -1, -2, -3, -4, 100., 200.}
		out := []byte{
			0x01, 0x02, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
			0xfe, 0xff, 0xfd, 0xff, 0xff, 0xff, 0xfc, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00,
			0xc8, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x69, 0x40,
		}
		compareLE(in, out)
	}()

	// slice
	func() {
		type t struct {
			A []int16
		}
		in := t{[]int16{1, 2, 3, 4}}
		out := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}
		compareLE(in, out)
	}()

	// slice with type conversion
	func() {
		type t struct {
			A []int `binary:"[]int8"`
		}
		in := t{[]int{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04}
		compareLE(in, out)
	}()

	// slice with type conversion and fixed size
	func() {
		type t struct {
			A []int `binary:"[8]int8"`
		}
		in := t{[]int{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		compareLE(in, out)
	}()

	// slice with explicit 'any' type
	func() {
		type t struct {
			A []int8 `binary:"[8]any"`
		}
		in := t{[]int8{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		compareLE(in, out)
	}()

	// slice with implicit 'any' type
	func() {
		type t struct {
			A []int8 `binary:"[8]"`
		}
		in := t{[]int8{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		compareLE(in, out)
	}()

	// array with type conversion
	func() {
		type t struct {
			A [4]int `binary:"[]int8"`
		}
		in := t{[4]int{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04}
		compareLE(in, out)
	}()

	// array with type conversion and fixed size
	func() {
		type t struct {
			A [4]int `binary:"[8]int8"`
		}
		in := t{[4]int{1, 2, 3, 4}}
		out := []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}
		compareLE(in, out)
	}()

	// zero bytes
	func() {
		type t struct {
			X interface{} `binary:"zero"`
			Y interface{} `binary:"zero(8)"`
			Z interface{} `binary:"[4]zero"`
		}
		in := t{}
		out := []byte{
			0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0,
		}
		compareLE(in, out)
	}()

	// string
	func() {
		type t struct {
			S string
		}
		in := t{"hello"}
		out := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f}
		compareLE(in, out)
	}()

	// string with size reference
	func() {
		type t struct {
			StringLen int16
			Str       string `binary:"string(StringLen+1)"`
		}
		s := "hello"
		in := t{int16(len(s)), s}
		out := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		compareLE(in, out)
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
		out := []byte{0x01, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00}
		compareLE(in, out)
	}()

	// bstring
	func() {
		type t struct {
			S string `binary:"bstring"`
		}
		in := t{"hello"}
		out := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		compareLE(in, out)

		// fixed buffer size bstring
		type t2 struct {
			S string `binary:"bstring(10)"`
		}
		in2 := t2{"hello"}
		out2 := []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		compareLE(in2, out2)
	}()

	// wstring
	func() {
		type t struct {
			S string `binary:"wstring"`
		}
		in := t{"hello"}
		out := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		compareLE(in, out)

		// fixed buffer size wstring
		type t2 struct {
			S string `binary:"wstring(10)"`
		}
		in2 := t2{"hello"}
		out2 := []byte{0x05, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		compareLE(in2, out2)
	}()

	// dwstring
	func() {
		type t struct {
			S string `binary:"dwstring"`
		}
		in := t{"hello"}
		out := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
		compareLE(in, out)

		// fixed buffer size dwstring
		type t2 struct {
			S string `binary:"dwstring(10)"`
		}
		in2 := t2{"hello"}
		out2 := []byte{0x05, 0x00, 0x00, 0x00, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0, 0, 0, 0, 0}
		compareLE(in2, out2)
	}()

	// string to []string
	func() {
		type t struct {
			S string `binary:"[3]string(0x10)"` // S matches to string[0]
		}
		in := t{"hello"}
		out := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		compareLE(in, out)

		type t2 struct {
			S string `binary:"[3]string(5)"`
		}
		in2 := t2{"hello"}
		out2 := []byte{
			0x68, 0x65, 0x6c, 0x6c, 0x6f,
			0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00,
		}
		compareLE(in2, out2)
	}()

	// string to []byte
	func() {
		type t struct {
			S string `binary:"[8]byte"`
		}
		in := t{"hello"}
		out := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00}
		compareLE(in, out)
	}()
	//printHex(res)

	// string to []int16
	func() {
		type t struct {
			S string `binary:"[8]int16"`
		}
		in := t{"hello"}
		out := []byte{0x68, 0, 0x65, 0, 0x6c, 0, 0x6c, 0, 0x6f, 0, 0x00, 0, 0x00, 0, 0x00, 0}
		compareLE(in, out)
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
		compareLE(sp, op)
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
		compareLE(si, oi)
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
		out := []byte{
			0x01, 0x02, 0x00, 0x01, 0x05, 0x00, 0x68, 0x65,
			0x6c, 0x6c, 0x6f, 0x68, 0x65, 0x6c, 0x6c, 0x6f,
			0x32, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03,
			0x04, 0x05, 0x00, 0x00, 0x00, 0x06, 0x00, 0x06,
			0x00, 0xa4, 0x70, 0x45, 0x41, 0x04, 0x03, 0x02,
			0x01,
		}
		compareLE(in, out)
	}()

}
