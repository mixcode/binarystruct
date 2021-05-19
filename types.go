package binarystruct

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// type ByteOrder is alias of binary.ByteOrder
type ByteOrder = binary.ByteOrder

var (
	BigEndian    binary.ByteOrder = binary.BigEndian
	LittleEndian binary.ByteOrder = binary.LittleEndian
)

// Type is name of types could be set to struct tag
type Type uint

const (
	Invalid Type = iota
	Int8
	Int16
	Int32
	Int64
	Uint8
	Uint16
	Uint32
	Uint64
	Float32
	Float64
	Byte      // byte (uint8)
	Word      // word (uint16)
	Dword     // double word (uint32)
	Qword     // quad word (uint16)
	Bstring   // {size Uint8, string [size]byte}
	Wstring   // {size Uint16, string [size]byte}
	Dwstring  // {size Uint32, string [size]byte}
	Zstring   // zero-terminated string
	Bzstring  // Bstring but zero-terminated
	Wzstring  // Wstring but zero-terminated
	Dwzstring // Dwstring but zero-terminated
	Zero      // zero bytes
	Struct    // a struct
	Any       // any type
)

var ErrInvalidType = fmt.Errorf("invalid type")

// get type value from its string name
func TypeByName(name string) Type {
	return typeMap[strings.ToLower(name)]
}

func (t Type) String() string {
	return typeNameMap[t]
}

func (t Type) ByteSize() int {
	p, ok := properties[t]
	if ok {
		return p.bytesize
	}
	return 0
}

// get internal kind type
func (t Type) iKind() iKind {
	p, ok := properties[t]
	if ok {
		return p.kind
	}
	return iKind(Invalid)
}

type typeOption struct {
	isArray  bool
	arrayLen int
	encoding string
}

func getNaturalType(v reflect.Value) (t Type, option typeOption) {
	kind := v.Kind()

	switch kind {

	// exact sized values
	case reflect.Int8:
		t = Int8
		return
	case reflect.Int16:
		t = Int16
		return
	case reflect.Int32:
		t = Int32
		return
	case reflect.Int64:
		t = Int64
		return

	case reflect.Uint8:
		t = Uint8
		return
	case reflect.Uint16:
		t = Uint16
		return
	case reflect.Uint32:
		t = Uint32
		return
	case reflect.Uint64:
		t = Uint64
		return

	case reflect.Float32:
		t = Float32
		return
	case reflect.Float64:
		t = Float64
		return

	// architecture-dependent sized values
	case reflect.Bool:
		t = Uint8
		return
	case reflect.Int:
		t = Int64
		return
	case reflect.Uint:
		t = Uint64
		return

	// non-scalar types
	case reflect.String:
		t = Wstring
		return
	case reflect.Struct:
		t = Struct
		return

	// array and slice
	case reflect.Array, reflect.Slice:
		elem := v.Type().Elem()
		k := elem.Kind()
		option.isArray = true
		if k == reflect.Array || k == reflect.Slice {
			t = Any
			return
		}
		option.arrayLen = v.Len()
		t, _ = getNaturalType(reflect.New(elem).Elem())
		return

	// indirect
	case reflect.Interface, reflect.Ptr:
		e := v.Elem()
		return getNaturalType(e)

		//default:
		//case reflect.Complex64, reflect.Complex128:
		//case reflect.Chan, reflect.Func, reflect.Map, reflect.UnsafePointer, reflect.Invalid
		//case reflect.Uintptr:
	}

	t = Invalid
	return
}

func decodeFunc(srcType Type, destRType reflect.Type) func(reflect.Value, uint64) error {

	printerr := func(v interface{}, t reflect.Value) error {
		return fmt.Errorf("valud %v not fit in type %v", v, t.Type())
	}

	// get destination size
	var srcKind iKind
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
		if srcKind == intKind || srcKind == uintKind {
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
		if srcKind == intKind || srcKind == uintKind {
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
		if srcKind == intKind {
			return func(v reflect.Value, u uint64) error {
				f := float64(int64(u))
				if v.OverflowFloat(f) {
					return printerr(f, v)
				}
				v.SetFloat(f)
				return nil
			}
		} else if srcKind == uintKind {
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
		if destKind == intKind && srcSize <= destSize {
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
		if destKind == uintKind && srcSize <= destSize {
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

type typenames struct {
	name string
	t    Type
}

// iKind of types
type iKind uint

const (
	invalidKind iKind = iota
	intKind
	uintKind
	floatKind
	stringKind
	structKind
	anyKind
)

var (
	// internal reflect types
	i64type = reflect.TypeOf(int64(0))
	f32type = reflect.TypeOf(float32(0.0))
	f64type = reflect.TypeOf(float64(0.0))

	// constants converted to to 64-bit images
	minInt8  = int64(math.MinInt8)
	minInt16 = int64(math.MinInt16)
	minInt32 = int64(math.MinInt32)
	minInt64 = int64(math.MinInt64)

	// properties of Kinds
	properties = map[Type]struct {
		kind     iKind
		bytesize int
		min, max uint64
	}{
		Invalid: {invalidKind, 0, 0, 0},

		Int8:  {intKind, 1, uint64(minInt8), uint64(math.MaxInt8)},
		Int16: {intKind, 2, uint64(minInt16), uint64(math.MaxInt16)},
		Int32: {intKind, 4, uint64(minInt32), uint64(math.MaxInt32)},
		Int64: {intKind, 8, uint64(minInt64), uint64(math.MaxInt64)},

		Uint8:  {uintKind, 1, 0, math.MaxUint8},
		Uint16: {uintKind, 2, 0, math.MaxUint16},
		Uint32: {uintKind, 4, 0, math.MaxUint32},
		Uint64: {uintKind, 8, 0, math.MaxUint64},

		Float32: {floatKind, 2, 0, 0},
		Float64: {floatKind, 4, 0, 0},

		Byte:  {uintKind, 1, 0, math.MaxUint8},
		Word:  {uintKind, 2, 0, math.MaxUint16},
		Dword: {uintKind, 4, 0, math.MaxUint32},
		Qword: {uintKind, 8, 0, math.MaxUint64},

		Zstring:  {stringKind, 0, 0, 0},
		Bstring:  {stringKind, 0, 0, 0},
		Wstring:  {stringKind, 0, 0, 0},
		Dwstring: {stringKind, 0, 0, 0},

		Bzstring:  {stringKind, 0, 0, 0},
		Wzstring:  {stringKind, 0, 0, 0},
		Dwzstring: {stringKind, 0, 0, 0},

		Any:    {anyKind, 0, 0, 0},
		Struct: {structKind, 0, 0, 0},
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
		{"Word", Word},
		{"Dword", Dword},
		{"Qword", Qword},
		{"Zstring", Zstring},
		{"Bstring", Bstring},
		{"Wstring", Wstring},
		{"DWString", Dwstring},
		{"BZString", Bstring},
		{"WZString", Wstring},
		{"DWZString", Dwstring},
		{"Zero", Zero},
		{"Struct", Struct},
		{"Any", Any},
	}
)

func init() {
	// init maps of kind constant and name
	for _, e := range kinds {
		typeMap[strings.ToLower(e.name)] = e.t
		typeNameMap[e.t] = e.name
	}
}
