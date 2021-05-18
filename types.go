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

	ErrInvalidKind = fmt.Errorf("invalid kind")

	i64type = reflect.TypeOf(int64(0))
)

type Kind uint

const (
	Invalid Kind = Kind(reflect.Invalid)
	Int8         = Kind(reflect.Int8)
	Int16        = Kind(reflect.Int16)
	Int32        = Kind(reflect.Int32)
	Int64        = Kind(reflect.Int64)
	Uint8        = Kind(reflect.Uint8)
	Uint16       = Kind(reflect.Uint16)
	Uint32       = Kind(reflect.Uint32)
	Uint64       = Kind(reflect.Uint64)
	Float32      = Kind(reflect.Float32)
	Float64      = Kind(reflect.Float64)

	Byte Kind = iota + 0x1000
	ZString
	BString
	WString
	DWString
	Zero
)

func GetKind(name string) Kind {
	return kindMap[strings.ToLower(name)]
}

func (k Kind) String() string {
	return kindNameMap[k]
}

func (k Kind) ByteSize() int {
	p, ok := properties[k]
	if ok {
		return p.bytesize
	}
	return 0
}

func decodeFunc(k reflect.Kind) func(reflect.Value, uint64) {
	switch k {

	case reflect.Bool:
		return func(v reflect.Value, u uint64) {
			v.SetBool(u > 0)
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(v reflect.Value, u uint64) {
			v.SetInt(int64(u))
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(v reflect.Value, u uint64) {
			v.SetUint(u)
		}

	case reflect.Float32:
		return func(v reflect.Value, u uint64) {
			v.SetFloat(float64(math.Float32frombits(uint32(u))))
		}

	case reflect.Float64:
		return func(v reflect.Value, u uint64) {
			v.SetFloat(math.Float64frombits(u))
		}
	}
	return nil
}

func encodeFunc(srcType reflect.Type, destKind Kind) func(reflect.Value) (uint64, int, error) {

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
					//err = fmt.Errorf("value not fit in %s", destKind)
					err = fmt.Errorf("value not fit in %s [%d, %d]", destKind, min, max)
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
			return func(v reflect.Value) (value uint64, bytesize int, err error) {
				u64 := v.Uint()
				if maxu64 < u64 {
					err = fmt.Errorf("value not fit in %s", destKind)
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

type kindnames struct {
	name string
	kind Kind
}

var (
	// constants converted to to 64-bit images
	minInt8  = int64(math.MinInt8)
	minInt16 = int64(math.MinInt16)
	minInt32 = int64(math.MinInt32)
	minInt64 = int64(math.MinInt64)

	// properties of Kinds
	properties = map[Kind]struct {
		bytesize int
		signed   bool
		min, max uint64
	}{
		Invalid: {0, false, 0, 0},
		Int8:    {1, true, uint64(minInt8), uint64(math.MaxInt8)},
		Int16:   {2, true, uint64(minInt16), uint64(math.MaxInt16)},
		Int32:   {4, true, uint64(minInt32), uint64(math.MaxInt32)},
		Int64:   {8, true, uint64(minInt64), uint64(math.MaxInt64)},
		Byte:    {1, false, 0, math.MaxUint8},
		Uint8:   {1, false, 0, math.MaxUint8},
		Uint16:  {2, false, 0, math.MaxUint16},
		Uint32:  {4, false, 0, math.MaxUint32},
		Uint64:  {8, false, 0, math.MaxUint64},
		Float32: {2, false, 0, 0},
		Float64: {4, false, 0, 0},
	}

	kindMap     = make(map[string]Kind)
	kindNameMap = make(map[Kind]string)

	kinds = []kindnames{
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
		kindMap[strings.ToLower(e.name)] = e.kind
		kindNameMap[e.kind] = e.name
	}
}
