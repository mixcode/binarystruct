package binarystruct

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	//"fmt"
	//"os"
	"io"
	"reflect"
)

var (
	ErrCannotSet  = errors.New("the value cannot be set")
	ErrNotAnArray = errors.New("must be an array or slice type")
)

// Unmarshal decodes binary images into data. Data should be a writable type such as a slice, a pointer or an interface.
func Unmarshal(input []byte, order binary.ByteOrder, data interface{}) (n int, err error) {
	buf := bytes.NewBuffer(input)
	return readValue(buf, order, reflect.ValueOf(data))
}

// Read reads binary structured data from r into data. Data should be a writable type such as a slice, a pointer or an interface.
func Read(r io.Reader, order binary.ByteOrder, data interface{}) (n int, err error) {
	return readValue(r, order, reflect.ValueOf(data))
}

// read a reflect.Value
func readValue(r io.Reader, order ByteOrder, v reflect.Value) (n int, err error) {
	t := v.Type()
	k := t.Kind()
	for k == reflect.Ptr || k == reflect.Interface {
		v = reflect.Indirect(v)
		t = v.Type()
		k = t.Kind()
	}
	encodeType, option := getNaturalType(v)
	return readMain(r, order, v, encodeType, option)
}

// read a value as given type
func readMain(r io.Reader, order ByteOrder, v reflect.Value, encodeType iType, option typeOption) (n int, err error) {
	// type was a pointer or an interface
	if option.indirectCount > 0 {
		for i := 0; i < option.indirectCount; i++ {
			v = v.Elem()
		}
	}

	if option.isArray {
		// read an array
		return readArray(r, order, v, encodeType, option)
	}

	//fmt.Printf("encodeType :%v\n", encodeType) //!!

	// based on individual type
	switch encodeType {

	case iStruct:
		return readStruct(r, order, v)

	case Pad: // padding zero bytes: `binary:"pad(10)"`
		l := option.bufLen
		if l == 0 {
			l = 1
		}
		skipBytes(r, l)
		return l, nil

	case Ignore: // ignoring value: `binary:"ignore"`
		return 0, nil

	case iInvalid:
		err = ErrInvalidType
		return
	}

	// based on kind group
	switch encodeType.iKind() {

	case intKind, uintKind, bitmapKind, floatKind:
		return readScalar(r, order, v, encodeType)

	case structKind:
		return readStruct(r, order, v)

	case stringKind:
		return readString(r, order, v, encodeType, option.bufLen, option.encoding)
	}

	err = fmt.Errorf("unknown type %s", encodeType)
	return
}

// read a scalar value
func readScalar(r io.Reader, order ByteOrder, v reflect.Value, k iType) (n int, err error) {
	if !v.CanSet() {
		err = ErrCannotSet
		return
	}
	sz, dec := decodeFunc(k, v.Type())
	u64, err := readU64(r, order, sz)
	if err != nil {
		return
	}
	return sz, dec(v, u64)
}

// follow pointers to the end
func dereferencePointer(p reflect.Value) reflect.Value {
	if !p.IsValid() {
		return p // not a valid reference
	}
	eType := p.Type()
	eKind := eType.Kind()
	for eKind == reflect.Ptr || eKind == reflect.Interface {
		if eKind == reflect.Ptr && p.IsNil() {
			// add a new value to the pointer
			newP := reflect.New(p.Elem().Type()) // allocate a new value and get its pointer
			p.Set(newP)                          // set the pointer
		}
		p = p.Elem()
		if !p.IsValid() {
			return p // not a valid reference
		}
		eType = p.Type()
		eKind = eType.Kind()
	}
	return p
}

func readSlice(r io.Reader, order ByteOrder, slice reflect.Value, elementType iType, option typeOption) (n int, err error) {

	arrayLen := option.arrayLen

	if slice.IsNil() {
		// make a new slice
		s := reflect.MakeSlice(slice.Type(), arrayLen, arrayLen)
		slice.Set(s)
	}
	sliceLen := slice.Len()
	readLen := arrayLen
	if arrayLen < sliceLen {
		readLen = sliceLen
	}

	loadSlice := func(uslice reflect.Value, l int) {
		var m int
		for i := 0; i < l; i++ {
			if elementType == Any {
				m, err = readValue(r, order, uslice.Index(i))
				if err != nil {
					return
				}
			} else {
				var o typeOption
				o.bufLen = option.bufLen     // option may contain inheritable values
				o.encoding = option.encoding // option may contain inheritable values
				m, err = readMain(r, order, uslice.Index(i), elementType, o)
				if err != nil {
					return
				}
			}
			n += m
		}
	}
	loadSlice(slice, readLen)
	if err != nil {
		return
	}

	if readLen < arrayLen {
		// original slice was too small
		newLen := arrayLen - readLen
		newSlice := reflect.MakeSlice(slice.Type(), newLen, newLen)
		loadSlice(newSlice, newLen)
		if err != nil {
			return
		}
		slice.Set(reflect.AppendSlice(slice, newSlice))
	}

	return
}

// read an array or slice
func readArray(r io.Reader, order ByteOrder, array reflect.Value, elementType iType, option typeOption) (n int, err error) {
	if option.arrayLen <= 0 {
		err = fmt.Errorf("array size unknown")
		return
	}
	arrayLen := option.arrayLen

	if elementType == Pad { // zero bytes
		// skip zero-byte types
		sz := option.bufLen
		if sz == 0 {
			sz = 1
		}
		sz *= arrayLen
		err = skipBytes(r, sz)
		if err != nil {
			return
		}
		return sz, nil
	}

	// deference a pointer or an interface
	array = dereferencePointer(array)
	eKind := array.Kind()

	if eKind == reflect.Slice {
		return readSlice(r, order, array, elementType, option)
	}
	if eKind != reflect.Array {
		err = ErrNotAnArray
		return
	}

	readLen := arrayLen
	if array.Len() < arrayLen {
		readLen = array.Len()
	}

	var m int
	for i := 0; i < readLen; i++ {
		if elementType == Any {
			m, err = readValue(r, order, array.Index(i))
			if err != nil {
				return
			}
		} else {
			var o typeOption
			o.bufLen = option.bufLen     // option may contain inheritable values
			o.encoding = option.encoding // option may contain inheritable values
			m, err = readMain(r, order, array.Index(i), elementType, o)
			if err != nil {
				return
			}
		}
		n += m
	}
	if readLen < arrayLen {
		// skip leftover bytes
		bytesz := elementType.ByteSize()
		if bytesz == 0 {
			err = fmt.Errorf("cannot determine element size")
			return
		}
		skipsz := (arrayLen - readLen) * bytesz
		err = skipBytes(r, skipsz)
		if err != nil {
			return
		}
		n += skipsz
	}

	/*
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

		var m int
		for i := 0; i < arrayLen; i++ {
			var e reflect.Value
			if arrayKind == reflect.Array || arrayKind == reflect.Slice {
				e = array.Index(i)
			} else {
				e = array
			}
			if elementType == iAny {
				m, err = writeValue(w, order, e)
				if err != nil {
					return
				}
			} else {
				var o typeOption
				o.bufLen = option.bufLen     // option may contain inheritable values
				o.encoding = option.encoding // option may contain inheritable values
				m, err = writeMain(w, order, e, elementType, o)
				if err != nil {
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
				for eKind == reflect.Ptr {
					eType = eType.Elem()
					eKind = eType.Kind()
				}
				sz = int(eType.Size())
			}

			// total size = element size * element count
			sz = sz * (desiredLen - arrayLen)

			// write blank bytes
			err = skipBytes(r, sz)
			if err != nil {
				return
			}
			n += sz
		}
	*/
	return
}

// read a struct
func readStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
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
		m, err = readMain(r, order, strc.Field(i), encodeType, option)
		if err != nil {
			err = wErr(i, err)
			return
		}
		n += m
	}
	return
}

// read string types
func readString(r io.Reader, order ByteOrder, v reflect.Value, encodeType iType, bufLen int, encoding string) (n int, err error) {
	/*
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

		// read string length
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
			// read string size header
			m, err = writeU64(w, order, uint64(strlen), headersz)
			if err != nil {
				return
			}
			n += m
		}

		// write string bytes
		m, err = r.Read([]byte(s))
		if err != nil {
			return
		}
		n += m

		if m < bufLen {
			// fill the leftovers
			sz := bufLen - m
			err = skipBytes(r, sz)
			if err != nil {
				return
			}
			n += sz
		}
	*/
	return
}

// read bytes according to the byte order
func readU64(r io.Reader, order ByteOrder, bytesize int) (u64 uint64, err error) {
	var buf [8]byte
	b := buf[:bytesize]
	_, err = io.ReadAtLeast(r, b, bytesize)
	if err != nil {
		return
	}
	switch bytesize {
	case 1:
		u64 = uint64(b[0])
	case 2:
		u64 = uint64(order.Uint16(b))
	case 4:
		u64 = uint64(order.Uint32(b))
	case 8:
		u64 = order.Uint64(b)
	default:
		panic("invalid byte size")
	}
	return
}

// skip blank bytes
func skipBytes(r io.Reader, sz int) (err error) {
	maxBufSize := 16384
	bsz := sz
	if bsz > maxBufSize {
		bsz = maxBufSize
	}
	buf := make([]byte, bsz)
	var m int
	for sz > 0 {
		if sz > maxBufSize {
			m, err = r.Read(buf)
		} else {
			m, err = r.Read(buf[:sz])
		}
		if err != nil {
			return
		}
		sz -= m
	}
	return
}
