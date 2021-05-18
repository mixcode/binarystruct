package binarystruct

import (
	//"bytes"

	// "os"
	"math"
	"reflect"
	"testing"
)

func TestEncodeFunc(t *testing.T) {
	//var err error

	testdata := []struct {
		k     Kind
		i     interface{}
		u     uint64
		isErr bool
	}{
		// Bool
		{Int8, bool(false), 0, false},
		{Int8, bool(true), 1, false},
		{Int16, bool(false), 0, false},
		{Int16, bool(true), 1, false},
		{Int32, bool(false), 0, false},
		{Int32, bool(true), 1, false},
		{Int64, bool(false), 0, false},
		{Int64, bool(true), 1, false},
		{Uint8, bool(false), 0, false},
		{Uint8, bool(true), 1, false},
		{Uint16, bool(false), 0, false},
		{Uint16, bool(true), 1, false},
		{Uint32, bool(false), 0, false},
		{Uint32, bool(true), 1, false},
		{Uint64, bool(false), 0, false},
		{Uint64, bool(true), 1, false},

		// int
		{Int8, int8(-1), 0xffffffff_ffffffff, false},
		{Int16, int16(-1), 0xffffffff_ffffffff, false},
		{Int32, int32(-1), 0xffffffff_ffffffff, false},
		{Int64, int64(-1), 0xffffffff_ffffffff, false},
		{Byte, byte(0xff), 0xff, false},
		{Uint8, uint8(0xff), 0xff, false},
		{Uint16, uint16(0xffff), 0xffff, false},
		{Uint32, uint32(0xffffffff), 0xffffffff, false},
		{Uint64, uint64(0xffffffff_ffffffff), 0xffffffff_ffffffff, false},

		// int range
		{Int8, uint64(math.MaxInt8), 0x7f, false},
		{Int8, int64(math.MinInt8), 0xffffffff_ffffff80, false},
		{Int8, uint64(math.MaxInt8 + 1), 0, true},
		{Int8, int64(math.MinInt8 - 1), 0, true},

		{Int16, uint64(math.MaxInt16), 0x7fff, false},
		{Int16, int64(math.MinInt16), 0xffffffff_ffff8000, false},
		{Int16, uint64(math.MaxInt16 + 1), 0, true},
		{Int16, int64(math.MinInt16 - 1), 0, true},

		{Int32, uint64(math.MaxInt32), 0x7fffffff, false},
		{Int32, int64(math.MinInt32), 0xffffffff_80000000, false},
		{Int32, uint64(math.MaxInt32 + 1), 0, true},
		{Int32, int64(math.MinInt32 - 1), 0, true},

		{Int64, uint64(math.MaxInt64), 0x7fffffff_ffffffff, false},
		{Int64, int64(math.MinInt64), 0x80000000_00000000, false},
		{Int64, uint64(math.MaxInt64 + 1), 0, true},

		// uint range
		{Uint8, uint64(math.MaxUint8), 0xff, false},
		{Uint8, 0, 0, false},
		{Uint8, int64(math.MaxUint8 + 1), 0, true},
		{Uint8, int(-1), 0, true},

		{Uint16, uint64(math.MaxUint16), 0xffff, false},
		{Uint16, 0, 0, false},
		{Uint16, int64(math.MaxUint16 + 1), 0, true},
		{Uint16, int(-1), 0, true},

		{Uint32, uint64(math.MaxUint32), 0xffffffff, false},
		{Uint32, 0, 0, false},
		{Uint32, int64(math.MaxUint32 + 1), 0, true},
		{Uint32, int(-1), 0, true},

		{Uint64, uint64(math.MaxUint64), 0xffffffff_ffffffff, false},
		{Uint64, 0, 0, false},
		{Uint64, int(-1), 0, true},

		// float
		{Float32, float32(12.3456), 0x41458794, false},
		{Float64, float64(12.3456), 0x4028b0f27bb2fec5, false},

		// float range
		{Float32, float64(123.5), 0x42f70000, false},
		{Float32, float64(12.3456), 0x41458794, false},
		{Int8, float64(12.3456), 12, false},
	}

	for _, e := range testdata {
		v := reflect.ValueOf(e.i)
		enc := encodeFunc(v.Type(), e.k)
		if enc == nil {
			t.Errorf("encoder for %s not found", v.Kind())
			continue
		}
		u, _, err := enc(v)
		if err != nil && !e.isErr {
			t.Error(err)
			continue
		}
		if err == nil {
			if e.isErr {
				t.Errorf("must produce an error: Kind %s, original %v, expected %x, encoded %x", e.k, e.i, e.u, u)
				continue
			}
			if u != e.u {
				t.Errorf("invalide encode value: Kind %s, original %v, expected %x, encoded %x", e.k, e.i, e.u, u)
				continue
			}
		}

		/*
			if err == nil {
				fmt.Printf("%s, %T, %v, %x\n", e.k, e.i, e.i, u)
			} else {
				fmt.Printf("%s, %T, %v, ERR\n", e.k, e.i, e.i)
			}
		*/

		/*
			// copy a value
			r := reflect.New(v.Type()).Elem()
			dec := decodeFunc(v.Kind())
			if dec == nil {
				t.Errorf("decoder for %s not found", v.Kind())
				continue
			}

			fmt.Printf("type: [%v]\n", r.Type())

			dec(r, u)
			if e.i != r.Interface() {
				t.Errorf("decoded value does not match: original %v, decoded %v", e.i, r.Interface())
			}
		*/

	}

}
