// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"io"
	"reflect"
)

var (
	// supplied value must be a pointer or an interface
	ErrCannotSet = errors.New("the value cannot be set")

	// the field is tagged as array but underlying value is not
	ErrNotAnArray = errors.New("must be an array or slice type")

	// cannot determine the length of an array or slice
	ErrUnknownLength = errors.New("unknown array, slice or string size")
)

// Unmarshal decodes binary images into a Go value. The Go value must be a writable type such as a slice, a pointer or an interface.
func Unmarshal(input []byte, order ByteOrder, govalue interface{}) (n int, err error) {
	var ms Marshaller
	return (&ms).Unmarshal(input, order, govalue)
}

// Read reads binary data from r and decode it into a Go value. The Go value must be a writable type such as a slice, a pointer or an interface.
func Read(r io.Reader, order ByteOrder, data interface{}) (n int, err error) {
	var ms Marshaller
	return (&ms).readValue(r, order, reflect.ValueOf(data))
}

// Unmarshaller.Unmarshal() is binary image decoder with environment in a Marshaller.
func (ms *Marshaller) Unmarshal(input []byte, order binary.ByteOrder, govalue interface{}) (n int, err error) {
	buf := bytes.NewBuffer(input)
	return ms.Read(buf, order, govalue)
}

// Unmarshaller.Read() is binary stream decoder with environment in a Marshaller.
func (ms *Marshaller) Read(r io.Reader, order binary.ByteOrder, data interface{}) (n int, err error) {
	return ms.readValue(r, order, reflect.ValueOf(data))
}

// read a reflect.Value
func (ms *Marshaller) readValue(r io.Reader, order ByteOrder, v reflect.Value) (n int, err error) {
	k := v.Type().Kind()
	if k == reflect.Ptr || k == reflect.Interface {
		v, _ = dereferencePointer(v)
	}
	encodeType, option := getNaturalType(v)
	return ms.readMain(r, order, v, encodeType, option)
}

// read a value as given type
func (ms *Marshaller) readMain(r io.Reader, order ByteOrder, v reflect.Value, encodeType eType, option typeOption) (n int, err error) {
	// type was a pointer or an interface
	if option.indirectCount > 0 {
		for i := 0; i < option.indirectCount; i++ {
			v = v.Elem()
		}
	}

	if option.isArray {
		// read an array
		return ms.readArray(r, order, v, encodeType, option)
	}

	// based on individual type
	switch encodeType {

	case iStruct:
		return ms.readStruct(r, order, v)

	case Pad: // padding zero bytes: `binary:"pad(10)"`
		l := option.bufLen
		if l == 0 {
			l = 1
		}
		return skipBytes(r, l)

	case Ignore: // ignoring value: `binary:"ignore"`
		return 0, nil

	case iInvalid:
		err = ErrInvalidType
		return
	}

	// based on kind group
	switch encodeType.iKind() {

	case intKind, uintKind, bitmapKind, floatKind:
		return ms.readScalar(r, order, v, encodeType)

	case structKind:
		return ms.readStruct(r, order, v)

	case stringKind:
		return ms.readString(r, order, v, encodeType, option.bufLen, option.encoding)
	}

	err = fmt.Errorf("unknown type %s", encodeType)
	return
}

func (ms *Marshaller) readSlice(r io.Reader, order ByteOrder, slice reflect.Value, elementType eType, option typeOption) (n int, err error) {

	if slice.Kind() != reflect.Slice {
		err = fmt.Errorf("must be a slice type; current type is %s", slice.Kind().String())
		return
	}
	arrayLen := option.arrayLen

	if slice.IsNil() {
		if arrayLen == 0 {
			// No data: return nil
			return
		}
		// make a new slice
		s := reflect.MakeSlice(slice.Type(), arrayLen, arrayLen)
		slice.Set(s)
	} else if arrayLen == 0 {
		// use existing slice
		arrayLen = slice.Len()
	}
	sliceLen := slice.Len()
	readLen := arrayLen
	if arrayLen < sliceLen {
		readLen = sliceLen
	}

	loadSlice := func(uslice reflect.Value, l int) {
		wErr := func(i int, e error) error {
			if i == 0 && e == io.EOF {
				// if EOF occurs at the first element, then the whole slice returns EOF
				return e
			}
			return fmt.Errorf("array index [%d]: %w", i, e)
		}
		var m int

		if uslice.Type().Elem().Kind() == reflect.Uint8 &&
			(elementType == Byte || elementType == Uint8) {
			// Byte slice. Note that reflect.Int8 is not a byte slice.
			b := uslice.Bytes()
			if len(b) < l {
				panic("invalid slice size")
			}
			m, err = io.ReadFull(r, b[:l])
			if err != nil {
				return
			}
			n += m

		} else {

			for i := 0; i < l; i++ {
				if elementType == Any {
					m, err = ms.readValue(r, order, uslice.Index(i))
				} else {
					var o typeOption
					o.bufLen = option.bufLen     // option may contain inheritable values
					o.encoding = option.encoding // option may contain inheritable values
					m, err = ms.readMain(r, order, uslice.Index(i), elementType, o)
				}
				n += m
				if err != nil {
					err = wErr(i, err)
					return
				}
			}
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
func (ms *Marshaller) readArray(r io.Reader, order ByteOrder, array reflect.Value, elementType eType, option typeOption) (n int, err error) {

	// deference a pointer or an interface
	array, _ = dereferencePointer(array)
	eKind := array.Kind()

	if eKind == reflect.Slice {
		return ms.readSlice(r, order, array, elementType, option)
	}

	// special case 1:
	// if the value is not an array but tagged as an array, treat it as the first element of an virtual array
	//	n int	`binary:[3]int8`	// n will be the first element of the array. other elements are ignored.
	destIsArray := (eKind == reflect.Array)

	arrayLen := option.arrayLen
	if arrayLen == 0 {
		if destIsArray {
			arrayLen = array.Len()
		} else {
			arrayLen = 1
		}
	}
	if arrayLen == 0 {
		// empty array: return immediately
		return
	}

	if elementType == Pad { // zero bytes
		// skip zero-byte types
		sz := option.bufLen
		if sz == 0 {
			sz = 1
		}
		sz *= arrayLen
		return skipBytes(r, sz)
	}

	var m int

	ik := elementType.iKind()
	if eKind == reflect.String && (ik == intKind || ik == uintKind || ik == bitmapKind || ik == floatKind) {
		// special case 2:
		// if the value is a string and the encoded type is array of numbers, then
		//	s string	`binary:[5]int8`	// 5-byte wide string
		buf := make([]byte, arrayLen)
		n, err = ms.readSlice(r, order, reflect.ValueOf(buf), elementType, option)
		if err != nil {
			return
		}

		//
		// TODO: string encoding (before removing terminating zeros)
		//

		l := len(buf)
		for ; l > 0 && buf[l-1] == 0; l-- {
			// skip terminating zeros
		}
		array.SetString(string(buf[:l]))
		return
	}

	readLen := arrayLen
	if !destIsArray {
		readLen = 1
	} else if array.Len() < arrayLen {
		readLen = array.Len()
	}

	wErr := func(i int, e error) error {
		if i == 0 && e == io.EOF {
			// if EOF occurs at the first element, then the whole slice returns EOF
			return e
		}
		return fmt.Errorf("array index [%d]: %w", i, e)
	}

	var v reflect.Value
	for i := 0; i < readLen; i++ {
		if !destIsArray {
			v = array
		} else {
			// normal array
			v = array.Index(i)
		}

		if elementType == Any {
			m, err = ms.readValue(r, order, v)
		} else {
			var o typeOption
			o.bufLen = option.bufLen     // option may contain inheritable values
			o.encoding = option.encoding // option may contain inheritable values
			m, err = ms.readMain(r, order, v, elementType, o)
		}
		n += m
		if err != nil {
			err = wErr(i, err)
			return
		}
	}
	if readLen < arrayLen {
		// skip leftover members
		bytesz := elementType.ByteSize()
		if bytesz != 0 { // fixed size value
			skipsz := (arrayLen - readLen) * bytesz
			m, err = skipBytes(r, skipsz)
			n += m
			return
		}
		// variable size value
		newv := reflect.New(v.Type()).Elem()
		o := typeOption{bufLen: option.bufLen, encoding: option.encoding}
		for i := readLen; i < arrayLen; i++ {
			m, err = ms.readMain(r, order, newv, elementType, o)
			n += m
			if err != nil {
				err = wErr(i, err)
				return
			}
		}
	}

	return
}

// read a struct
func (ms *Marshaller) readStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
	typ := strc.Type()
	nField := typ.NumField()

	firstElem := true
	wErr := func(i int, e error) error { // return a wrapped error
		if firstElem {
			// If EOF occurs at the first non-ignoring field, then return a raw EOF
			return e
		}
		f := typ.Field(i)
		return fmt.Errorf("field#%d <%s>: %w", i, f.Name, e)
	}
	for i := 0; i < nField; i++ {
		// Read tag info if available
		encodeType, option, e := parseStructField(typ, strc, i)
		if e != nil {
			err = wErr(i, e)
			return
		}

		if encodeType == Ignore { // `binary:"ignore"` or `binary:"-"`
			continue
		}

		f := typ.Field(i)
		name := f.Name
		if len(name) == 0 || strings.ToUpper(name)[0] != name[0] {
			// unexported type
			continue
		}

		fKind := f.Type.Kind()
		v := strc.Field(i)
		if fKind == reflect.Ptr || fKind == reflect.Interface {
			// allocate pointers
			v, _ = dereferencePointer(v)
			option.indirectCount = 0
		}

		var m int
		m, err = ms.readMain(r, order, v, encodeType, option)
		if err != nil {
			err = wErr(i, err)
			return
		}
		n += m
		firstElem = false
	}
	return
}

// read a zero-terminating bytes.
// str is actual non-zero bytes. readsz may be len(str)+1.
func readZString(r io.Reader) (str []byte, readsz int, err error) {
	str = make([]byte, 0)
	if br, ok := r.(io.ByteReader); ok {
		// has a bytereader
		var b byte
		for {
			b, err = br.ReadByte()
			if err != nil {
				return
			}
			readsz++
			if b == 0 {
				break
			}
			str = append(str, b)
		}
	} else {
		bb := make([]byte, 1)
		for {
			_, err = r.Read(bb)
			if err != nil {
				return
			}
			readsz++
			if bb[0] == 0 {
				break
			}
			str = append(str, bb[0])
		}
	}
	return
}

// read a two-zero-terminating bytes.
// may be a uint16/UTF16 bytes contained in a []byte stream.
// str is actual bytes excluding final two zeros. readsz may be len(str)+2.
func readZ16String(r io.Reader) (str []byte, readsz int, err error) {
	str = make([]byte, 0)
	even := false
	var prevb byte
	if br, ok := r.(io.ByteReader); ok {
		// has a bytereader
		var b byte
		for {
			b, err = br.ReadByte()
			if err != nil {
				return
			}
			readsz++
			if even && prevb == 0 && b == 0 {
				break
			}
			str = append(str, b)
			prevb = b
			even = !even
		}
	} else {
		bb := make([]byte, 1)
		for {
			_, err = r.Read(bb)
			if err != nil {
				return
			}
			readsz++
			if even && prevb == 0 && bb[0] == 0 {
				break
			}
			str = append(str, bb[0])
			prevb = bb[0]
			even = !even
		}
	}
	return
}

// read string types
func (ms *Marshaller) readString(r io.Reader, order ByteOrder, v reflect.Value, encodeType eType, bufLen int, textEncoding string) (n int, err error) {

	switch encodeType {
	case Zstring: // zero-terminated byte string
		var buf []byte
		buf, n, err = readZString(r)
		if err != nil {
			return
		}
		// process text encoding
		strlen := len(buf)
		if textEncoding != "" && strlen > 0 {
			buf, err = ms.decodeText(buf, textEncoding)
			if err != nil {
				return
			}
			strlen = len(buf)
		}
		// remove additional terminaing zeros
		for ; strlen > 0 && buf[strlen-1] == 0; strlen-- {
			// empty
		}
		v.SetString(string(buf[:strlen]))
		return

	case Z16string: // zero-terminated UTF16 string
		var buf []byte
		buf, n, err = readZ16String(r)
		if err != nil {
			return
		}
		// process text encoding
		strlen := len(buf)
		if textEncoding != "" && strlen > 0 {
			buf, err = ms.decodeText(buf, textEncoding)
			if err != nil {
				return
			}
			strlen = len(buf)
		}
		// remove additional terminaing zeros
		for ; strlen > 0 && buf[strlen-1] == 0; strlen-- {
			// empty
		}
		v.SetString(string(buf[:strlen]))
		return
	}

	// Size-given string
	// read string length
	headersz := 0
	switch encodeType {
	case Bstring:
		headersz = 1
	case Wstring:
		headersz = 2
	case Dwstring:
		headersz = 4
	}

	strlen := 0
	if headersz > 0 {
		var u64 uint64
		u64, n, err = readU64(r, order, headersz)
		if err != nil {
			return
		}
		strlen = int(int64(u64))
	} else {
		strlen = bufLen
	}

	readsz := strlen
	if readsz < bufLen {
		// both data length and buffer size exists
		readsz = bufLen
	}

	buf := make([]byte, readsz)
	m := 0
	if readsz > 0 {
		m, err = io.ReadFull(r, buf)
		n += m
		if err != nil {
			return
		}
	}

	// process text encoding (before removing terminating zeros)
	if textEncoding != "" && m > 0 {
		buf, err = ms.decodeText(buf, textEncoding)
		if err != nil {
			return
		}
		strlen = len(buf)
	}

	// remove terminaing zeros if buffer is larger than actual string
	for ; strlen > 0 && buf[strlen-1] == 0; strlen-- {
		// empty
	}

	v.SetString(string(buf[:strlen]))
	return
}

// read bytes according to the byte order
func readU64(r io.Reader, order ByteOrder, bytesize int) (u64 uint64, n int, err error) {
	var buf [8]byte
	b := buf[:bytesize]
	n, err = io.ReadFull(r, b)
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

// read a scalar value
func (ms *Marshaller) readScalar(r io.Reader, order ByteOrder, v reflect.Value, k eType) (n int, err error) {
	if !v.CanSet() {
		err = ErrCannotSet
		return
	}
	sz, dec := decodeFunc(k, v.Type())
	u64, n, err := readU64(r, order, sz)
	if err != nil {
		return
	}
	err = dec(v, u64)
	return
}

// follow pointers to the end
func dereferencePointer(p reflect.Value) (value reflect.Value, indirectCount int) {
	if !p.IsValid() {
		return // not a valid reference
	}
	eType := p.Type()
	eKind := eType.Kind()
	for eKind == reflect.Ptr || eKind == reflect.Interface {
		indirectCount++
		if eKind == reflect.Ptr && p.IsNil() {
			// add a new value to the pointer
			newP := reflect.New(eType.Elem()) // allocate a new value and get its pointer
			p.Set(newP)                       // set the pointer
		}
		p = p.Elem()
		if !p.IsValid() {
			value = p
			return // not a valid reference
		}
		eType = p.Type()
		eKind = eType.Kind()
	}
	value = p
	return
}

// skip padding bytes
func skipBytes(r io.Reader, sz int) (n int, err error) {
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
		n += m
		sz -= m
		if err != nil {
			return
		}
	}
	return
}
