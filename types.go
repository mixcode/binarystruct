package binarystruct

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
)

type ByteOrder binary.ByteOrder

var (
	BigEndian    ByteOrder = binary.BigEndian
	LittleEndian ByteOrder = binary.LittleEndian

	ErrInvalidType = fmt.Errorf("invalid type")

	i64type = reflect.TypeOf(int64(0))
	f32type = reflect.TypeOf(float32(0.0))
	f64type = reflect.TypeOf(float64(0.0))
)

type Type uint

const (
	Invalid Type = Type(reflect.Invalid)
	Int8         = Type(reflect.Int8)
	Int16        = Type(reflect.Int16)
	Int32        = Type(reflect.Int32)
	Int64        = Type(reflect.Int64)
	Uint8        = Type(reflect.Uint8)
	Uint16       = Type(reflect.Uint16)
	Uint32       = Type(reflect.Uint32)
	Uint64       = Type(reflect.Uint64)
	Float32      = Type(reflect.Float32)
	Float64      = Type(reflect.Float64)

	Byte Type = iota + 0x1000
	ZString
	BString
	WString
	DWString
	Zero
)

func GetType(name string) Type {
	return typeMap[strings.ToLower(name)]
}

func (k Type) String() string {
	return typeNameMap[k]
}

func (k Type) ByteSize() int {
	p, ok := properties[k]
	if ok {
		return p.bytesize
	}
	return 0
}

func decodeFunc(srcType Type, destRType reflect.Type) func(reflect.Value, uint64) error {

	printerr := func(v interface{}, t reflect.Value) error {
		return fmt.Errorf("valud %v not fit in type %v", v, t.Type())
	}

	// get destination size
	var srcKind Kind
	if p, ok := properties[srcType]; ok {
		srcKind = p.kind
	}

	destRKind := destRType.Kind()
	switch destRKind {
	case reflect.Bool:
		return func(v reflect.Value, u uint64) error {
			v.SetBool(u != 0)
			return nil
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if srcKind == Int || srcKind == Uint {
			return func(v reflect.Value, u uint64) error {
				n := int64(u)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(int64(u))
				return nil
			}
		} else if srcType == Float32 {
			return func(v reflect.Value, u uint64) error {
				f := math.Float32frombits(uint32(u))
				n := int64(f)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(n)
				return nil
			}
		} else if srcType == Float64 {
			return func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				n := int64(f)
				if v.OverflowInt(n) {
					return printerr(n, v)
				}
				v.SetInt(n)
				return nil
			}
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if srcKind == Int || srcKind == Uint {
			return func(v reflect.Value, u uint64) error {
				if v.OverflowUint(u) {
					return printerr(u, v)
				}
				v.SetUint(u)
				return nil
			}
		} else if srcType == Float32 {
			return func(v reflect.Value, u uint64) error {
				f := math.Float32frombits(uint32(u))
				n := uint64(f)
				if v.OverflowUint(n) {
					return printerr(n, v)
				}
				v.SetUint(n)
				return nil
			}
		} else if srcType == Float64 {
			return func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				n := uint64(f)
				if v.OverflowUint(n) {
					return printerr(n, v)
				}
				v.SetUint(n)
				return nil
			}
		}

	case reflect.Float32, reflect.Float64:
		if srcKind == Int {
			return func(v reflect.Value, u uint64) error {
				f := float64(int64(u))
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
		} else if srcKind == Uint {
			return func(v reflect.Value, u uint64) error {
				f := float64(u)
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
		} else if srcType == Float32 {
			return func(v reflect.Value, u uint64) error {
				f := float64(math.Float32frombits(uint32(u)))
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
		} else if srcType == Float64 {
			return func(v reflect.Value, u uint64) error {
				f := math.Float64frombits(u)
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
		}
	}

	return nil
}

// Single value encoder
func encodeFunc(srcRType reflect.Type, destType Type) func(reflect.Value) (uint64, int, error) {

	printErrNotFit := func(v interface{}, t Type) error {
		return fmt.Errorf("value %v not fit in %s", v, t)
	}

	// get destination size
	var destSize int
	var minu64, maxu64 uint64
	var destKind Kind
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
		if destKind == Int && srcSize <= destSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(v.Int()), destSize, nil
			}
		} else if destKind == Int || destKind == Uint {
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
		if destKind == Uint && srcSize <= destSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return v.Uint(), destSize, nil
			}
		} else if destKind == Int || destKind == Uint {
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
		case Float32:
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(math.Float32bits(float32(v.Float()))), destSize, nil
			}
		case Float64:
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

/*
func decodeFunc(srcKind Kind, destType reflect.Type) func(reflect.Value, uint64) (error) {

	// get destination size
	var targetSize int
	var targetSigned bool
	var minu64, maxu64 uint64
	p, ok := properties[destKind]
	if ok {
		targetSize, targetSigned = p.bytesize, p.signed
		minu64, maxu64 = p.min, p.max
	}

	sk := srcType.Kind()
	switch sk {
	case reflect.Bool:
		return func(v reflect.Value) (value uint64, bytesize int, err error) {
			if v.Bool() {
				value = 1
			}
			bytesize = targetSize
			return
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		srcSize := int(srcType.Size())
		if targetSigned && srcSize <= targetSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(v.Int()), targetSize, nil
			}
		} else {
			// source value may not fit in the destination
			min, max := int64(minu64), int64(maxu64)
			if max < 0 { // destKind is Uint64 and it never overflows for int64
				max = math.MaxInt64
			}
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				i64 := v.Int()
				if i64 < min || max < i64 {
					err = fmt.Errorf("value %v not fit in %s", i64, destKind)
					return
				}
				return uint64(i64), targetSize, nil
			}
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		srcSize := int(srcType.Size())
		if !targetSigned && srcSize <= targetSize {
			// source value always fits in the destination
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return v.Uint(), targetSize, nil
			}
		} else {
			// source value may not fit in the destination
			//max := maxu64
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				u64 := v.Uint()
				if maxu64 < u64 {
					err = fmt.Errorf("value %v not fit in %s", u64, destKind)
					return
				}
				return u64, targetSize, nil
			}
		}

	case reflect.Float32, reflect.Float64:
		switch destKind {
		case Float32:
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return uint64(math.Float32bits(float32(v.Float()))), targetSize, nil
			}
		case Float64:
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return math.Float64bits(v.Float()), targetSize, nil
			}
		default:
			ec := encodeFunc(i64type, destKind)
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				return ec(v.Convert(i64type))
			}
		}

	}

	return nil
}
*/

type typenames struct {
	name string
	t    Type
}

type Kind uint

const (
	InvalidKind Kind = iota
	Int
	Uint
	Float
	String
)

var (
	// constants converted to to 64-bit images
	minInt8  = int64(math.MinInt8)
	minInt16 = int64(math.MinInt16)
	minInt32 = int64(math.MinInt32)
	minInt64 = int64(math.MinInt64)

	// properties of Kinds
	properties = map[Type]struct {
		kind     Kind
		bytesize int
		min, max uint64
	}{
		Invalid: {InvalidKind, 0, 0, 0},

		Int8:  {Int, 1, uint64(minInt8), uint64(math.MaxInt8)},
		Int16: {Int, 2, uint64(minInt16), uint64(math.MaxInt16)},
		Int32: {Int, 4, uint64(minInt32), uint64(math.MaxInt32)},
		Int64: {Int, 8, uint64(minInt64), uint64(math.MaxInt64)},

		Byte:   {Uint, 1, 0, math.MaxUint8},
		Uint8:  {Uint, 1, 0, math.MaxUint8},
		Uint16: {Uint, 2, 0, math.MaxUint16},
		Uint32: {Uint, 4, 0, math.MaxUint32},
		Uint64: {Uint, 8, 0, math.MaxUint64},

		Float32: {Float, 2, 0, 0},
		Float64: {Float, 4, 0, 0},

		ZString:  {String, 0, 0, 0},
		BString:  {String, 0, 0, 0},
		WString:  {String, 0, 0, 0},
		DWString: {String, 0, 0, 0},
		Zero:     {Uint, 1, 0, 0},
	}

	typeMap     = make(map[string]Type)
	typeNameMap = make(map[Type]string)

	kinds = []typenames{
		{"Invalid", Invalid},
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
		{"ZString", ZString},
		{"BString", BString},
		{"WString", WString},
		{"DWString", DWString},
		{"Zero", Zero},
	}
)

func init() {
	// init maps of kind constant and name
	for _, e := range kinds {
		typeMap[strings.ToLower(e.name)] = e.t
		typeNameMap[e.t] = e.name
	}
}
