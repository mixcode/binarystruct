// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// type ByteOrder is an alias of encoding/binary.ByteOrder
type ByteOrder = binary.ByteOrder

var (
	// byte orders
	BigEndian    binary.ByteOrder = binary.BigEndian
	LittleEndian binary.ByteOrder = binary.LittleEndian
)

// Name of encoding binary types recoginized in the struct field tags.
type eType uint

const (
	iInvalid eType = iota // internal invalid type

	//
	// Encoding types for struct field tags.
	// Note that type names in tags are case insensitive.
	//
	//
	// Signed values. Vaules must respect its valid range.
	// e.g.) Int8 must be in [-128, 127].
	Int8  // `binary:"int8"`
	Int16 // `binary:"int16"`
	Int32 // `binary:"int32"`
	Int64 // `binary:"int64"`
	//
	// Unsigned values. Must respect its valid range.
	// e.g.) Uint8 must be in [0, 255].
	Uint8  // `binary:"uint8"`
	Uint16 // `binary:"uint16"`
	Uint32 // `binary:"uint32"`
	Uint64 // `binary:"uint64"`
	//
	// Sign-agnostic bitmaps.
	// Be careful that these types are ambiguous in type conversion.
	// i.e) int `binary:"byte"` could map both 255 and -1 to 0xff.
	Byte  // 8-bit byte. `binary: "byte"`
	Word  // 16-bit word. `binary: "word"`
	Dword // 32-bit double word. `binary: "dword"`
	Qword // 64-bit quad word. `binary: "qword"`
	//
	// Floating point values.
	Float32 // `binary:"float32"`
	Float64 // `binary:"float64"`
	//
	// String types.
	// When string types are postfixed by '(size)'
	// then the encoded size will be exactly size bytes long.
	String   // []byte. `binary:"string"` `binary:"string(size)"`
	Bstring  // {size Uint8, string [size]byte}  `binary:"bstring"`
	Wstring  // {size Uint16, string [size]byte} `binary:"wstring"`
	Dwstring // {size Uint32, string [size]byte} `binary:"dwstring"`
	// zero-terminated string types.
	Zstring   // zero-terminated byte string, or C-style string. `binary:"zstring"`
	Z16string // zero-word-terminated word string. `binary:"z16string"`
	//z32string	// zero-dword-terminated dword string of unknown length.

	// struct type
	iStruct // internal struct type

	// misc types
	//

	// Pad is padding zero bytes. Original value is ignored.
	// May be postfixed by '(size)' to set number of bytes.
	// e.g.) `binary:"pad(0x8)"`
	Pad

	// Values with Ignore tag are ignored. `binary:"ignore"`
	Ignore

	// If a field is tagged with Any, or no tag is set,
	// then the the value's default encoding will be used.
	Any

	// internal-only types
	iArray // used in getNaturalType()
)

var (
	// possibly an invalid encoding type appears in a tag
	ErrInvalidType = errors.New("invalid binary type")
)

// get type value from its string name
func typeByName(name string) eType {
	return typeMap[strings.ToLower(name)]
}

// string representation of the type
func (t eType) String() string {
	return typeNameMap[t]
}

// Byte size of the type. If the type is not a scalar type (like slice), this function returns zero
func (t eType) ByteSize() int {
	p, ok := properties[t]
	if ok {
		return p.bytesize
	}
	return 0
}

// get internal kind type
func (t eType) iKind() iKind {
	p, ok := properties[t]
	if ok {
		return p.kind
	}
	return iKind(iInvalid)
}

type typeOption struct {
	indirectCount int    // if nonzero, original type is a pointer/interface and this value cotains the number of indirections to the actual value
	isArray       bool   // if true, tagged field is an array: `binary:"[arrayLen]TYPE"`
	arrayLen      int    // length of the tagged array
	bufLen        int    // tagged field is a string or a padding of length bufLen: `binary:"STRINGTYPE(buflen)"`
	encoding      string // string encoding of the field: `binary:"string,encoding=ENC"`
}

func getITypeFromRType(rt reflect.Type) (it eType) {
	switch rt.Kind() {

	// exact sized values
	case reflect.Int8:
		return Int8
	case reflect.Int16:
		return Int16
	case reflect.Int32:
		return Int32
	case reflect.Int64:
		return Int64

	case reflect.Uint8:
		return Uint8
	case reflect.Uint16:
		return Uint16
	case reflect.Uint32:
		return Uint32
	case reflect.Uint64:
		return Uint64

	case reflect.Float32:
		return Float32
	case reflect.Float64:
		return Float64

	// architecture-dependent sized values
	case reflect.Bool:
		return Uint8
	case reflect.Int:
		return Int64
	case reflect.Uint:
		return Uint64

	// non-scalar types
	case reflect.String:
		return String
	case reflect.Struct:
		return iStruct

	// array and slice
	case reflect.Array, reflect.Slice:
		return iArray

	case reflect.Ptr, reflect.Interface:
		return Any
	}
	return iInvalid
}

func getNaturalType(v reflect.Value) (t eType, option typeOption) {

	typ := v.Type()
	kind := typ.Kind()

	isNil := false
	for kind == reflect.Ptr || kind == reflect.Interface {
		option.indirectCount++
		if v.IsNil() {
			if kind == reflect.Interface {
				// can't determine the type of nil interface
				t = Any
				return
			}
			typ = typ.Elem()
			kind = typ.Kind()
			isNil = true
		} else {
			v = v.Elem()
			typ = v.Type()
			kind = typ.Kind()
		}
	}

	t = getITypeFromRType(typ)

	if t == iArray {
		elementType := typ.Elem()
		k := elementType.Kind()
		option.isArray = true
		if k == reflect.Array || k == reflect.Slice {
			t = Any
			return
		}
		if !isNil {
			option.arrayLen = v.Len()
		}
		t = getITypeFromRType(elementType)
	}

	return
}

// decodeFunc() generates a binary-type to go-type conversion function
func decodeFunc(srcType eType, destRType reflect.Type) (bytesz int, decoder func(reflect.Value, uint64) error) {

	printerr := func(v interface{}, t reflect.Value) error {
		return fmt.Errorf("value %v not fit in type %v", v, t.Type())
	}

	// get destination size
	var srcKind iKind
	if p, ok := properties[srcType]; ok {
		srcKind = p.kind
		bytesz = p.bytesize
	}

	destRKind := destRType.Kind()
	switch destRKind {
	case reflect.Bool:
		decoder = func(v reflect.Value, u uint64) error {
			v.SetBool(u != 0)
			return nil
		}
		return

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if srcKind == intKind || srcKind == bitmapKind {
			decoder = func(v reflect.Value, u uint64) error {
				var n int64
				switch bytesz { // sign extension
				case 1:
					n = int64(int8(u))
				case 2:
					n = int64(int16(u))
				case 4:
					n = int64(int32(u))
				case 8:
					n = int64(u)
				default:
					panic("invalid byte size")
				}
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(int64(u))
				return nil
			}
			return
		} else if srcKind == uintKind {
			decoder = func(v reflect.Value, u uint64) error {
				n := int64(u)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(int64(u))
				return nil
			}
			return
		} else if srcType == Float32 {
			decoder = func(v reflect.Value, u uint64) error {
				f := math.Float32frombits(uint32(u))
				n := int64(f)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(n)
				return nil
			}
			return
		} else if srcType == Float64 {
			decoder = func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				n := int64(f)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(n)
				return nil
			}
			return
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if srcKind == intKind {
			decoder = func(v reflect.Value, u uint64) error {
				var n int64
				switch bytesz { // sign extension
				case 1:
					n = int64(int8(u))
				case 2:
					n = int64(int16(u))
				case 4:
					n = int64(int32(u))
				case 8:
					n = int64(u)
				default:
					panic("invalid byte size")
				}
				if n < 0 {
					return printerr(n, v)
				}
				u = uint64(n)
				if v.OverflowUint(u) {
					return printerr(u, v)
				}
				v.SetUint(u)
				return nil
			}
			return
		} else if srcKind == uintKind || srcKind == bitmapKind {
			decoder = func(v reflect.Value, u uint64) error {
				if v.OverflowUint(u) {
					return printerr(u, v)
				}
				v.SetUint(u)
				return nil
			}
			return
		} else if srcType == Float32 {
			decoder = func(v reflect.Value, u uint64) error {
				f := math.Float32frombits(uint32(u))
				n := uint64(f)
				if v.OverflowUint(n) {
					return printerr(n, v)
				}
				v.SetUint(n)
				return nil
			}
			return
		} else if srcType == Float64 {
			decoder = func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				n := uint64(f)
				if v.OverflowUint(n) {
					return printerr(n, v)
				}
				v.SetUint(n)
				return nil
			}
			return
		}

	case reflect.Float32, reflect.Float64:
		if srcKind == intKind {
			decoder = func(v reflect.Value, u uint64) error {
				f := float64(int64(u))
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
			return
		} else if srcKind == uintKind {
			decoder = func(v reflect.Value, u uint64) error {
				f := float64(u)
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
			return
		} else if srcType == Float32 || srcType == Dword {
			decoder = func(v reflect.Value, u uint64) error {
				f := float64(math.Float32frombits(uint32(u)))
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
			return
		} else if srcType == Float64 || srcType == Qword {
			decoder = func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
			return
		}
	}

	return 0, nil
}

// encodeFunc() generates a go-type to binary-type conversion function
func encodeFunc(srcRType reflect.Type, destType eType) func(reflect.Value) (uint64, int, error) {

	printErrNotFit := func(v interface{}, t eType) error {
		return fmt.Errorf("value %v not fit in %s", v, t)
	}

	// get destination size
	var destSize int
	var minu64, maxu64 uint64
	var destKind iKind
	p, ok := properties[destType]
	if ok {
		destSize, destKind = p.bytesize, p.kind
		minu64, maxu64 = p.min, p.max
	}

	sk := srcRType.Kind()
	switch sk {

	case reflect.Bool:
		if destType == Float32 {
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				f := float32(0)
				if v.Bool() {
					f = 1.0
				}
				return uint64(math.Float32bits(f)), destSize, nil
			}
		}
		if destType == Float64 {
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				f := float64(0)
				if v.Bool() {
					f = 1.0
				}
				return math.Float64bits(f), destSize, nil
			}
		}
		return func(v reflect.Value) (value uint64, bytesize int, err error) {
			if v.Bool() {
				value = 1
			}
			bytesize = destSize
			return
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		srcSize := int(srcRType.Size())
		if destKind == bitmapKind { // byte/word/dword/qword
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				if int64(v.Type().Size()) < int64(destSize) {
					// only byte size matters
					err = printErrNotFit(value, destType)
					return
				}
				return uint64(v.Int()), destSize, nil
			}
		} else if destKind == intKind && srcSize <= destSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(v.Int()), destSize, nil
			}
		} else if destKind == intKind || destKind == uintKind {
			// source value may not fit in the destination
			min, max := int64(minu64), int64(maxu64)
			if max < 0 { // destKind is Uint64 and it never overflows for int64
				max = math.MaxInt64
			}
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				i64 := v.Int()
				if i64 < min || max < i64 {
					err = printErrNotFit(i64, destType)
					return
				}
				return uint64(i64), destSize, nil
			}
		} else if destType == Float32 {
			// convert integer to float32
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(math.Float32bits(float32(v.Convert(f32type).Float()))), destSize, nil
			}
		} else if destType == Float64 {
			// convert integer to float64
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return math.Float64bits(v.Convert(f64type).Float()), destSize, nil
			}
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		srcSize := int(srcRType.Size())
		if destKind == bitmapKind { // byte/word/dword/qword
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				if int64(v.Type().Size()) < int64(destSize) {
					// only byte size matters
					err = printErrNotFit(value, destType)
					return
				}
				return v.Uint(), destSize, nil
			}
		} else if destKind == uintKind && srcSize <= destSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return v.Uint(), destSize, nil
			}
		} else if destKind == intKind || destKind == uintKind {
			// source value may not fit in the destination
			//max := maxu64
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				u64 := v.Uint()
				if maxu64 < u64 {
					err = printErrNotFit(u64, destType)
					return
				}
				return u64, destSize, nil
			}
		} else if destType == Float32 {
			// convert unsigned integer to float32
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(math.Float32bits(float32(v.Convert(f32type).Float()))), destSize, nil
			}
		} else if destType == Float64 {
			// convert unsigned to float64
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return math.Float64bits(v.Convert(f64type).Float()), destSize, nil
			}
		}

	case reflect.Float32, reflect.Float64:
		switch destType {
		case Float32, Dword:
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(math.Float32bits(float32(v.Float()))), destSize, nil
			}
		case Float64, Qword:
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return math.Float64bits(v.Float()), destSize, nil
			}
		default:
			ec := encodeFunc(i64type, destType)
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return ec(v.Convert(i64type))
			}
		}

	}

	return nil
}

// internal kind of types
type iKind uint

const (
	invalidKind iKind = iota
	intKind           // signed number
	uintKind          // unsigned number
	bitmapKind        // type-agnosic bits
	floatKind         // floating point value
	stringKind        // string
	structKind        // struct
	anyKind           // other types
)

var (
	// internal reflect types
	i64type       = reflect.TypeOf(int64(0))
	f32type       = reflect.TypeOf(float32(0.0))
	f64type       = reflect.TypeOf(float64(0.0))
	byteSliceType = reflect.TypeOf(make([]byte, 0))

	// constants converted to to 64-bit images
	minInt8  = int64(math.MinInt8)
	minInt16 = int64(math.MinInt16)
	minInt32 = int64(math.MinInt32)
	minInt64 = int64(math.MinInt64)

	// properties of Kinds
	properties = map[eType]struct {
		kind     iKind
		bytesize int
		min, max uint64
	}{
		iInvalid: {invalidKind, 0, 0, 0},

		Int8:  {intKind, 1, uint64(minInt8), uint64(math.MaxInt8)},
		Int16: {intKind, 2, uint64(minInt16), uint64(math.MaxInt16)},
		Int32: {intKind, 4, uint64(minInt32), uint64(math.MaxInt32)},
		Int64: {intKind, 8, uint64(minInt64), uint64(math.MaxInt64)},

		Uint8:  {uintKind, 1, 0, math.MaxUint8},
		Uint16: {uintKind, 2, 0, math.MaxUint16},
		Uint32: {uintKind, 4, 0, math.MaxUint32},
		Uint64: {uintKind, 8, 0, math.MaxUint64},

		Byte:  {bitmapKind, 1, uint64(minInt8), math.MaxUint8},
		Word:  {bitmapKind, 2, uint64(minInt16), math.MaxUint16},
		Dword: {bitmapKind, 4, uint64(minInt32), math.MaxUint32},
		Qword: {bitmapKind, 8, uint64(minInt64), math.MaxUint64},

		Float32: {floatKind, 4, 0, 0},
		Float64: {floatKind, 8, 0, 0},

		String:    {stringKind, 0, 0, 0},
		Bstring:   {stringKind, 0, 0, 0},
		Wstring:   {stringKind, 0, 0, 0},
		Dwstring:  {stringKind, 0, 0, 0},
		Zstring:   {stringKind, 0, 0, 0},
		Z16string: {stringKind, 0, 0, 0},

		Pad:     {uintKind, 0, 0, 0},
		iStruct: {structKind, 0, 0, 0},
		Any:     {anyKind, 0, 0, 0},

		Ignore: {anyKind, 0, 0, 0},

		iArray: {anyKind, 0, 0, 0}, // internal only
	}
)

type typenames struct {
	name string
	t    eType
}

var (
	typeMap     = make(map[string]eType)
	typeNameMap = make(map[eType]string)

	kinds = []typenames{
		{"Invalid", iInvalid},
		{"Int8", Int8},
		{"Int16", Int16},
		{"Int32", Int32},
		{"Int64", Int64},
		{"Uint8", Uint8},
		{"Uint16", Uint16},
		{"Uint32", Uint32},
		{"Uint64", Uint64},
		{"Float32", Float32},
		{"Float64", Float64},
		{"Byte", Byte},
		{"Word", Word},
		{"Dword", Dword},
		{"Qword", Qword},
		{"String", String},
		{"Bstring", Bstring},
		{"Wstring", Wstring},
		{"DWString", Dwstring},
		{"Zstring", Zstring},
		{"Z16string", Z16string},
		{"Pad", Pad},
		{"Struct", iStruct},
		{"Any", Any},
		{"Ignore", Ignore},
	}
)

func init() {
	// init maps of kind constant and name
	for _, e := range kinds {
		typeMap[strings.ToLower(e.name)] = e.t
		typeNameMap[e.t] = e.name
	}
}
