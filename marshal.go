package binarystruct

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
)

var (
	mTag = regexp.MustCompile(`^\s*(\[([^\]]+)\])?([^\s]*)`)
)

// write a value to writer w. returns
func Write(w io.Writer, order ByteOrder, data interface{}) (n int, err error) {
	return writeValue(w, order, reflect.ValueOf(data))
}

// write a value
func writeValue(w io.Writer, order ByteOrder, v reflect.Value) (n int, err error) {
	t := v.Type()
	k := t.Kind()
	for k == reflect.Ptr || k == reflect.Interface {
		v = reflect.Indirect(v)
		t = v.Type()
		k = t.Kind()
	}
	encodeType, option := getNaturalType(v)

	return writeMain(w, order, v, encodeType, option)
}

// write a value as given type
func writeMain(w io.Writer, order ByteOrder, v reflect.Value, encodeType Type, option typeOption) (n int, err error) {

	if option.isArray {
		// write the array
		if option.arrayLen == 0 {
			return
		}
		return writeArray(w, order, v, encodeType, option)
	}

	if encodeType == Struct {
		return writeStruct(w, order, v)
	}

	if encodeType == Zstring {
		return writeZeroTerminatedString(w, v, option.arrayLen, option.encoding)
	}

	encodeKind := encodeType.iKind()
	switch encodeKind {

	case intKind, uintKind, floatKind:
		return writeScalar(w, order, v, encodeType)

	case structKind:
		return writeStruct(w, order, v)

	case stringKind:
		return writeString(w, order, v, encodeType, option.encoding)
	}
	err = fmt.Errorf("NOT IMPLEMENTED YET")
	return
}

// write a scalar value
func writeScalar(w io.Writer, order ByteOrder, v reflect.Value, k Type) (n int, err error) {
	enc := encodeFunc(v.Type(), k)
	if enc == nil {
		err = ErrInvalidType
		return
	}
	u64, sz, err := enc(v)
	if err != nil {
		return
	}
	return writeU64(w, order, u64, sz)
}

// write an array
func writeArray(w io.Writer, order ByteOrder, array reflect.Value, elementType Type, option typeOption) (n int, err error) {
	l := array.Len()
	var m int
	for i := 0; i < l; i++ {
		e := array.Index(i)
		if elementType == Any {
			m, err = writeValue(w, order, e)
			if err != nil {
				return
			}
		} else {
			var o typeOption
			o.encoding = option.encoding // option may contain inheritable values
			m, err = writeMain(w, order, e, elementType, o)
			if err != nil {
				return
			}
		}
		n += m
	}
	return
}

// write a struct
func writeStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	typ := strc.Type()
	nField := typ.NumField()
	for i := 0; i < nField; i++ {
		// Read tag info if available
		encodeType, option, e := parseStructField(typ, strc, i)
		if e != nil {
			err = e
			return
		}

		var m int
		m, err = writeMain(w, order, strc.Field(i), encodeType, option)
		if err != nil {
			return
		}
		n += m
	}
	return
}

// write string types except Zstring
func writeString(w io.Writer, order ByteOrder, v reflect.Value, encodeType Type, encoding string) (n int, err error) {
	s := v.String()

	terminateZero := false
	if encodeType == Zstring || encodeType == Bzstring || encodeType == Wzstring || encodeType == Dwzstring {
		terminateZero = true
	}

	var m int

	//
	// TODO: process encoding
	//

	// write string length
	l := uint64(len(s))
	maxlen, bytesz := uint64(0), 0
	switch encodeType {
	case Bstring, Bzstring:
		maxlen, bytesz = math.MaxUint8, 1
	case Wstring, Wzstring:
		maxlen, bytesz = math.MaxUint16, 2
	case Dwstring, Dwzstring:
		maxlen, bytesz = math.MaxUint32, 4
	}
	if terminateZero {
		l++
	}
	if l > maxlen {
		err = fmt.Errorf("string too long: len %d, max %d", l, maxlen)
		return
	}
	m, err = writeU64(w, order, l, bytesz)
	if err != nil {
		return
	}
	n += m

	// write string bytes
	m, err = w.Write([]byte(s))
	if err != nil {
		return
	}
	n += m

	if terminateZero {
		m, err = w.Write([]byte{0})
		if err != nil {
			return
		}
		n += m
	}
	return
}

// write a zero-terminated string as [buflen]byte
func writeZeroTerminatedString(w io.Writer, v reflect.Value, buflen int, encoding string) (n int, err error) {
	s := v.String()
	l := len(s)
	if l > buflen {
		err = fmt.Errorf("string too long: len %d, max %d", l, buflen)
		return
	}

	//
	// TODO: process encoding
	//

	// write string bytes
	m, err := w.Write([]byte(s))
	if err != nil {
		return
	}
	n += m
	// fill buffer leftover
	j := buflen - m
	if j == 0 {
		return
	}
	blank := make([]byte, j)
	m, err = w.Write(blank)
	if err != nil {
		return
	}
	n += m
	return
}

// write bytes according to the byte order
func writeU64(w io.Writer, order ByteOrder, u64 uint64, bytesize int) (n int, err error) {
	var buf [8]byte
	b := buf[:bytesize]
	switch bytesize {
	case 1:
		b[0] = byte(u64)
	case 2:
		order.PutUint16(b, uint16(u64))
	case 4:
		order.PutUint32(b, uint32(u64))
	case 8:
		order.PutUint64(b, u64)
	default:
		panic("invalid byte size")
	}
	return w.Write(b)
}
