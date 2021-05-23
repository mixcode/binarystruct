// Copyright 2021 mixcode@github

package binarystruct

import (
	"math"
	"reflect"
	"testing"
)

// TestScalarValueEncoding() tests encoding and decoding of integer and float unit types
func TestScalarValueEncoding(t *testing.T) {
	//var err error

	testdata := []struct {
		typ       eType       // Encoding target type
		value     interface{} // Value to be encoded
		u64       uint64      // Encoded value. u64 ← typ(value)
		encodeErr bool        // encoding will generate an error
		decodable bool        // decoding will generate the same value with the original value
	}{
		// Bool
		{Int8, bool(false), 0, false, true},
		{Int8, bool(true), 1, false, true},
		{Int16, bool(false), 0, false, true},
		{Int16, bool(true), 1, false, true},
		{Int32, bool(false), 0, false, true},
		{Int32, bool(true), 1, false, true},
		{Int64, bool(false), 0, false, true},
		{Int64, bool(true), 1, false, true},
		{Uint8, bool(false), 0, false, true},
		{Uint8, bool(true), 1, false, true},
		{Uint16, bool(false), 0, false, true},
		{Uint16, bool(true), 1, false, true},
		{Uint32, bool(false), 0, false, true},
		{Uint32, bool(true), 1, false, true},
		{Uint64, bool(false), 0, false, true},
		{Uint64, bool(true), 1, false, true},
		{Float32, bool(false), 0, false, true},
		{Float32, bool(true), 0x3f800000, false, true},
		{Float64, bool(false), 0, false, true},
		{Float64, bool(true), 0x3ff00000_00000000, false, true},

		// int/uint
		{Int8, int8(-1), 0xffffffff_ffffffff, false, true},
		{Int16, int16(-1), 0xffffffff_ffffffff, false, true},
		{Int32, int32(-1), 0xffffffff_ffffffff, false, true},
		{Int64, int64(-1), 0xffffffff_ffffffff, false, true},
		{Uint8, uint8(0xff), 0xff, false, true},
		{Uint16, uint16(0xffff), 0xffff, false, true},
		{Uint32, uint32(0xffffffff), 0xffffffff, false, true},
		{Uint64, uint64(0xffffffff_ffffffff), 0xffffffff_ffffffff, false, true},

		// unspecified type
		{Byte, int8(-1), 0xffffffff_ffffffff, false, true},
		{Word, int16(-1), 0xffffffff_ffffffff, false, true},
		{Dword, int32(-1), 0xffffffff_ffffffff, false, true},
		{Qword, int64(-1), 0xffffffff_ffffffff, false, true},
		{Byte, uint8(0xff), 0xff, false, true},
		{Word, uint16(0xffff), 0xffff, false, true},
		{Dword, uint32(0xffffffff), 0xffffffff, false, true},
		{Qword, uint64(0xffffffff_ffffffff), 0xffffffff_ffffffff, false, true},

		// int range
		{Int8, uint64(math.MaxInt8), 0x7f, false, true},
		{Int8, int64(math.MinInt8), 0xffffffff_ffffff80, false, true},
		{Int8, uint64(math.MaxInt8 + 1), 0, true, false},
		{Int8, int64(math.MinInt8 - 1), 0, true, false},

		{Int16, uint64(math.MaxInt16), 0x7fff, false, true},
		{Int16, int64(math.MinInt16), 0xffffffff_ffff8000, false, true},
		{Int16, uint64(math.MaxInt16 + 1), 0, true, false},
		{Int16, int64(math.MinInt16 - 1), 0, true, false},

		{Int32, uint64(math.MaxInt32), 0x7fffffff, false, true},
		{Int32, int64(math.MinInt32), 0xffffffff_80000000, false, true},
		{Int32, uint64(math.MaxInt32 + 1), 0, true, false},
		{Int32, int64(math.MinInt32 - 1), 0, true, false},

		{Int64, uint64(math.MaxInt64), 0x7fffffff_ffffffff, false, true},
		{Int64, int64(math.MinInt64), 0x80000000_00000000, false, true},
		{Int64, uint64(math.MaxInt64 + 1), 0, true, false},

		// uint range
		{Uint8, uint64(math.MaxUint8), 0xff, false, true},
		{Uint8, 0, 0, false, true},
		{Uint8, int64(math.MaxUint8 + 1), 0, true, false},
		{Uint8, int(-1), 0, true, false},

		{Uint16, uint64(math.MaxUint16), 0xffff, false, true},
		{Uint16, 0, 0, false, true},
		{Uint16, int64(math.MaxUint16 + 1), 0, true, false},
		{Uint16, int(-1), 0, true, false},

		{Uint32, uint64(math.MaxUint32), 0xffffffff, false, true},
		{Uint32, 0, 0, false, true},
		{Uint32, int64(math.MaxUint32 + 1), 0, true, false},
		{Uint32, int(-1), 0, true, false},

		{Uint64, uint64(math.MaxUint64), 0xffffffff_ffffffff, false, true},
		{Uint64, 0, 0, false, true},
		{Uint64, int(-1), 0, true, false},

		// float-to-int
		{Int8, float64(127.9), 0x7f, false, false},
		{Int8, float64(128.9), 0, true, false},
		{Int16, float32(32767.9), 0x7fff, false, false},
		{Int16, float32(32768), 0, true, false},
		{Int32, float64(float64(math.MaxInt32) + 0.9), 0x7fffffff, false, false},
		{Int32, float64(float64(math.MaxInt32) + 1), 0, true, false},
		{Int64, float64(123456*100000 + 0.789), 0x2_dfdae800, false, false},
		{Int64, float64(math.MaxInt64 + 999), 0x80000000_00000000, false, false},

		// int-to-float
		{Float32, uint8(math.MaxUint8), 0x437f0000, false, true},
		{Float32, uint16(math.MaxUint16), 0x477fff00, false, true},
		{Float32, uint32(math.MaxUint32), 0x4f800000, false, false}, // decoded value will overflow uint32
		{Float32, uint64(math.MaxUint64), 0x5f800000, false, false}, // decoded value will not match
		{Float64, int8(math.MinInt8), 0xc0600000_00000000, false, true},
		{Float64, int16(math.MinInt16), 0xc0e00000_00000000, false, true},
		{Float64, int32(math.MinInt32), 0xc1e00000_00000000, false, true},
		{Float64, int64(math.MinInt64), 0xc3e00000_00000000, false, true},

		// float
		{Float32, float32(12.3456), 0x41458794, false, true},
		{Float64, float64(12.3456), 0x4028b0f27bb2fec5, false, true},
		{Float32, float64(123.5), 0x42f70000, false, true},
		{Float64, float32(12.3456), 0x4028b0f280000000, false, false}, // decoded data will be 12.345600128173828
		{Dword, float32(12.3456), 0x41458794, false, true},
		{Qword, float64(12.3456), 0x4028b0f27bb2fec5, false, true},
	}

	for _, e := range testdata {

		//
		// encode value to binary
		//
		v := reflect.ValueOf(e.value)
		enc := encodeFunc(v.Type(), e.typ)
		if enc == nil {
			t.Errorf("encoder for %s not found", v.Kind())
			continue
		}
		u, sz, err := enc(v)
		if err != nil {
			if !e.encodeErr {
				t.Error(err)
			}
			continue
		}
		if sz != e.typ.ByteSize() {
			t.Errorf("byte size does not match: Kind %s, required %d, actual %d", e.typ, e.typ.ByteSize(), sz)
		}
		if err == nil {
			if e.encodeErr {
				t.Errorf("must produce an error: Kind %s, original %v, expected 0x%x, encoded 0x%x", e.typ, e.value, e.u64, u)
				continue
			}
			if u != e.u64 {
				t.Errorf("invalide encode value: Kind %s, original %v, expected %x, encoded %x", e.typ, e.value, e.u64, u)
				continue
			}
		}

		/*
			// print encoding results
			if err == nil {
				fmt.Printf("%s, %T, %v, %x\n", e.typ, e.value, e.value, u)
			} else {
				fmt.Printf("%s, %T, %v, ERR\n", e.typ, e.value, e.value)
			}
		*/

		if !e.decodable {
			continue
		}

		//
		// decode binary to value
		//
		r := reflect.New(v.Type()).Elem() // prepare a receiver for the decoded value
		decsz, dec := decodeFunc(e.typ, v.Type())
		if dec == nil {
			t.Errorf("decoder for %s not found", v.Kind())
			continue
		}
		if decsz != sz {
			t.Errorf("decoding byte size(%d) and encoding size(%d) does not match", decsz, sz)
			continue
		}

		err = dec(r, u)
		if err != nil {
			t.Error(err)
			continue
		}
		if e.value != r.Interface() {
			t.Errorf("decoded value does not match: image 0x%x, original %v, decoded %v", u, e.value, r.Interface())
			continue
		}

	}

}
