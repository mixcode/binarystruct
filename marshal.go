package binarystruct

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
)

// Marshal encodes data into a binary image and return it as []byte.
func Marshal(data interface{}, order ByteOrder) ([]byte, error) {
	var b bytes.Buffer
	_, err := writeValue(&b, order, reflect.ValueOf(data))
	return b.Bytes(), err
}

// Write encodes data into binary stream and writes to r.
func Write(w io.Writer, order ByteOrder, data interface{}) (n int, err error) {
	return writeValue(w, order, reflect.ValueOf(data))
}

// write a reflect.Value
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
func writeMain(w io.Writer, order ByteOrder, v reflect.Value, encodeType iType, option typeOption) (n int, err error) {

	// type was a pointer or an interface
	if option.indirectCount > 0 {
		for i := 0; i < option.indirectCount; i++ {
			v = v.Elem()
		}
	}

	if option.isArray {
		// write the array
		if option.arrayLen == 0 {
			return
		}
		return writeArray(w, order, v, encodeType, option)
	}

	// based on individual type
	switch encodeType {

	case iStruct:
		return writeStruct(w, order, v)

	case Pad: // padding zero bytes: `binary:"pad(10)"`
		l := option.bufLen
		if l == 0 {
			l = 1
		}
		return zeroFill(w, l)

	case Ignore: // ignoring value: `binary:"ignore"`
		return 0, nil

	case iInvalid:
		err = ErrInvalidType
		return
	}

	// based on kind group
	switch encodeType.iKind() {

	case intKind, uintKind, bitmapKind, floatKind:
		return writeScalar(w, order, v, encodeType)

	case structKind:
		return writeStruct(w, order, v)

	case stringKind:
		return writeString(w, order, v, encodeType, option.bufLen, option.encoding)
	}

	err = fmt.Errorf("unknown type %s", encodeType)
	return
}

// write a scalar value
func writeScalar(w io.Writer, order ByteOrder, v reflect.Value, k iType) (n int, err error) {
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
func writeArray(w io.Writer, order ByteOrder, array reflect.Value, elementType iType, option typeOption) (n int, err error) {

	arrayKind := array.Kind()
	//
	// Go arrays and slices are primary target of array notation.
	//	a []int	`binary:"[10]byte"`
	// And there is a special case for string.
	//	s string `binary:"[10]uint16"`	// each string byte is converted to uint16
	// An exceptional case is that the target type is string array and given value is a string.
	//	s string `binary:"[3]zstring(0x10)"`	// s is writen as first string, and the others will be blank string
	//
	if arrayKind == reflect.String && elementType.iKind() != stringKind {
		// convert string to byte slice
		array = array.Convert(byteSliceType)
		arrayKind = array.Kind()
	}

	arrayLen := 1
	if arrayKind == reflect.Array || arrayKind == reflect.Slice {
		arrayLen = array.Len()
	}

	desiredLen := option.arrayLen
	if desiredLen <= 0 {
		desiredLen = arrayLen
	}
	if desiredLen < arrayLen {
		err = fmt.Errorf("array too large to fit: len %d, size %d", desiredLen, arrayLen)
		return
		// arrayLen = desiredLen
	}

	wErr := func(i int, e error) error {
		return fmt.Errorf("array index [%d]: %w", i, e)
	}
	var m int
	for i := 0; i < arrayLen; i++ {
		var e reflect.Value
		if arrayKind == reflect.Array || arrayKind == reflect.Slice {
			e = array.Index(i)
		} else {
			e = array
		}
		if elementType == Any {
			m, err = writeValue(w, order, e)
			if err != nil {
				err = wErr(i, err)
				return
			}
		} else {
			var o typeOption
			o.bufLen = option.bufLen     // option may contain inheritable values
			o.encoding = option.encoding // option may contain inheritable values
			m, err = writeMain(w, order, e, elementType, o)
			if err != nil {
				err = wErr(i, err)
				return
			}
		}
		n += m
	}
	if arrayLen < desiredLen {
		// fill the leftover
		sz := option.bufLen // element length supplied
		if sz == 0 {
			sz = m // m holds the byte count of last written element
		}
		if sz == 0 {
			// guess byte size of the element type
			eType := array.Elem().Type()
			eKind := eType.Kind()
			for eKind == reflect.Ptr || eKind == reflect.Interface {
				eType = eType.Elem()
				eKind = eType.Kind()
			}
			sz = int(eType.Size())
		}

		// total size = element size * element count
		sz = sz * (desiredLen - arrayLen)

		// write blank bytes
		m, err = zeroFill(w, sz)
		n += m
		if err != nil {
			return
		}
	}
	return
}

// write a struct
func writeStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	typ := strc.Type()
	nField := typ.NumField()
	wErr := func(i int, e error) error {
		f := typ.Field(i)
		return fmt.Errorf("field <%s>: %w", f.Name, e)
	}
	for i := 0; i < nField; i++ {
		// Read tag info if available
		encodeType, option, e := parseStructField(typ, strc, i)
		if e != nil {
			err = wErr(i, e)
			return
		}

		if encodeType == Ignore { // `binary:"ignore"`
			continue
		}
		name := typ.Field(i).Name
		if len(name) == 0 || strings.ToUpper(name)[0] != name[0] {
			// unexported type
			continue
		}

		var m int
		m, err = writeMain(w, order, strc.Field(i), encodeType, option)
		if err != nil {
			err = wErr(i, err)
			return
		}
		n += m
	}
	return
}

// write string types
func writeString(w io.Writer, order ByteOrder, v reflect.Value, encodeType iType, bufLen int, encoding string) (n int, err error) {
	s := v.String()

	var m int

	//
	// TODO: process string encoding
	//

	strlen := len(s)
	if bufLen <= 0 {
		bufLen = strlen
	}
	if bufLen < strlen {
		err = fmt.Errorf("string too long: len %d, buffer size %d", strlen, bufLen)
		return
	}

	// write string length
	maxlen, headersz := uint64(math.MaxInt64), 0
	switch encodeType {
	case Bstring:
		maxlen, headersz = math.MaxUint8, 1
	case Wstring:
		maxlen, headersz = math.MaxUint16, 2
	case Dwstring:
		maxlen, headersz = math.MaxUint32, 4
	}
	if uint64(bufLen) > maxlen {
		err = fmt.Errorf("string too long: len %d, max %d", strlen, maxlen)
		return
	}

	if headersz > 0 {
		// write string size header
		m, err = writeU64(w, order, uint64(strlen), headersz)
		n += m
		if err != nil {
			return
		}
	}

	// write string bytes
	m, err = w.Write([]byte(s))
	n += m
	if err != nil {
		return
	}

	if m < bufLen {
		// fill the leftovers
		return zeroFill(w, bufLen-m)
	}
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

// write blank padding bytes
func zeroFill(w io.Writer, sz int) (n int, err error) {
	maxBufSize := 16384
	bsz := sz
	if bsz > maxBufSize {
		bsz = maxBufSize
	}
	buf := make([]byte, bsz)
	var m int
	for sz > 0 {
		if sz > maxBufSize {
			m, err = w.Write(buf)
		} else {
			m, err = w.Write(buf[:sz])
		}
		n += m
		sz -= m
		if err != nil {
			return
		}
	}
	return
}
