// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"bytes"
	"errors"
	"fmt"

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

	// validation constraint failed
	ErrValidationError = errors.New("validation failed")

	// no byte order is available to encode/decode a multi-byte value: the value
	// declared none (no struct-level or per-field endian=) and the Marshaler has
	// no Order. Declare it on the struct via a blank _ struct{} field tagged
	// endian=, or use NewMarshalerOrder(order) / set Marshaler.Order.
	errNoByteOrder = errors.New("no byte order: declare endian= on the struct or use NewMarshalerOrder(order)")
)

// DecodeError is returned when unmarshalling fails, describing the field name and byte offset of the failure.
type DecodeError struct {
	Offset int
	Field  string
	Err    error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("decode error at offset %d (field %s): %v", e.Offset, e.Field, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

// Unmarshal decodes binary images into a Go value. The Go value must be a writable type such as a slice, a pointer or an interface.
func Unmarshal(input []byte, govalue interface{}) (n int, err error) {
	return NewMarshaler().Unmarshal(input, govalue)
}

// Read reads binary data from r and decode it into a Go value. The Go value must be a writable type such as a slice, a pointer or an interface.
func Read(r io.Reader, data interface{}) (n int, err error) {
	return NewMarshaler().Read(r, data)
}

// UnmarshalAs decodes binary images into a Go value using the supplied tag. The Go value must be a writable type such as a slice, a pointer or an interface.
func UnmarshalAs(input []byte, tag string, govalue interface{}) (n int, err error) {
	return NewMarshaler().UnmarshalAs(input, tag, govalue)
}

// ReadAs reads binary data from r and decodes it into a Go value using the supplied tag. The Go value must be a writable type such as a slice, a pointer or an interface.
func ReadAs(r io.Reader, tag string, data interface{}) (n int, err error) {
	return NewMarshaler().ReadAs(r, tag, data)
}

// Marshaler.Unmarshal() decodes binary data into a Go value using the Marshaler's byte order.
func (ms *Marshaler) Unmarshal(input []byte, govalue interface{}) (n int, err error) {
	buf := bytes.NewBuffer(input)
	return ms.Read(buf, govalue)
}

// Marshaler.UnmarshalAs() decodes binary data using the supplied tag and the Marshaler's byte order.
func (ms *Marshaler) UnmarshalAs(input []byte, tag string, govalue interface{}) (n int, err error) {
	buf := bytes.NewBuffer(input)
	return ms.ReadAs(buf, tag, govalue)
}

// Marshaler.Read() decodes a binary stream into a Go value. The byte order comes
// from the value's declaration, falling back to the Marshaler's Order field.
func (ms *Marshaler) Read(r io.Reader, data interface{}) (n int, err error) {
	return ms.readValue(r, ms.Order, reflect.ValueOf(data))
}

// Marshaler.ReadAs() decodes a binary stream using the supplied tag.
func (ms *Marshaler) ReadAs(r io.Reader, tag string, data interface{}) (n int, err error) {
	order := ms.Order
	v := reflect.ValueOf(data)
	k := v.Type().Kind()
	if k == reflect.Ptr || k == reflect.Interface {
		v, _ = dereferencePointer(v)
	}

	var fieldErr error
	switch v.Kind() {
	case reflect.Invalid:
		fieldErr = fmt.Errorf("invalid data type")
	case reflect.Complex64, reflect.Complex128:
		fieldErr = fmt.Errorf("complex type not supported")
	case reflect.UnsafePointer:
		fieldErr = fmt.Errorf("pointer type not supported")
	case reflect.Chan, reflect.Func, reflect.Map:
		fieldErr = fmt.Errorf("unsupported type: %v", v.Kind())
	}

	naturalType, naturalOption := getNaturalType(v)
	encodeType, option, err := parseTagString(tag, reflect.Value{}, naturalType, naturalOption, fieldErr)
	if err != nil {
		return 0, err
	}

	return ms.readMain(r, order, v, encodeType, option, reflect.Value{}, -1)
}

// read a reflect.Value
func (ms *Marshaler) readValue(r io.Reader, order ByteOrder, v reflect.Value) (n int, err error) {
	k := v.Type().Kind()
	if k == reflect.Ptr || k == reflect.Interface {
		v, _ = dereferencePointer(v)
	}
	encodeType, option := getNaturalType(v)
	return ms.readMain(r, order, v, encodeType, option, reflect.Value{}, -1)
}

// read a value as given type
func (ms *Marshaler) readMain(r io.Reader, order ByteOrder, v reflect.Value, encodeType eType, option typeOption, parentStruct reflect.Value, fieldIndex int) (n int, err error) {
	order = resolveByteOrder(order, option.endian)

	if option.codec != "" {
		codec, ok := ms.codecs[option.codec]
		if !ok {
			return 0, fmt.Errorf("unknown codec: %s", option.codec)
		}
		val, n, err := codec.Decode(r, parentStruct, fieldIndex, order)
		if err != nil {
			return n, err
		}
		if !v.CanSet() {
			return n, ErrCannotSet
		}
		valVal := reflect.ValueOf(val)
		if valVal.Type().AssignableTo(v.Type()) {
			v.Set(valVal)
		} else if valVal.Type().ConvertibleTo(v.Type()) {
			v.Set(valVal.Convert(v.Type()))
		} else {
			return n, fmt.Errorf("cannot assign deserialized value of type %T to field of type %s", val, v.Type().String())
		}
		return n, nil
	}

	if encodeType == Any {
		var naturalOption typeOption
		encodeType, naturalOption = getNaturalType(v)
		option.indirectCount += naturalOption.indirectCount
		// Adopt the natural array length when the tag left it unknown (e.g. an
		// untagged nested Go array), mirroring writeMain.
		if naturalOption.isArray && option.arrayLen == 0 && len(option.dims) == 0 {
			option.isArray = true
			option.arrayLen = naturalOption.arrayLen
		}
	}

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

func (ms *Marshaler) readSlice(r io.Reader, order ByteOrder, slice reflect.Value, elementType eType, option typeOption) (n int, err error) {

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
					m, err = ms.readMain(r, order, uslice.Index(i), elementType, o, reflect.Value{}, -1)
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
func (ms *Marshaler) readArray(r io.Reader, order ByteOrder, array reflect.Value, elementType eType, option typeOption) (n int, err error) {

	// deference a pointer or an interface
	array, _ = dereferencePointer(array)

	// Multidimensional tag (e.g. [4][2]int8): read each outer element as a
	// sub-array of the remaining dimensions. Recurse through readMain so the
	// innermost dimension falls through to the normal slice/array path below.
	if len(option.dims) > 1 {
		child := option
		child.dims = option.dims[1:]
		child.arrayLen = child.dims[0]
		child.isArray = true
		outerLen := option.dims[0]
		switch array.Kind() {
		case reflect.Slice:
			if outerLen <= 0 {
				if array.IsNil() {
					return 0, nil // implicit outer length with no destination: nothing to read
				}
				outerLen = array.Len()
			}
			if array.IsNil() || array.Len() < outerLen {
				array.Set(reflect.MakeSlice(array.Type(), outerLen, outerLen))
			}
		case reflect.Array:
			if outerLen <= 0 || outerLen > array.Len() {
				outerLen = array.Len()
			}
		default:
			return 0, fmt.Errorf("multidimensional binary tag on non-array value of kind %s", array.Kind())
		}
		var m int
		for i := 0; i < outerLen; i++ {
			m, err = ms.readMain(r, order, array.Index(i), elementType, child, reflect.Value{}, -1)
			n += m
			if err != nil {
				if i == 0 && err == io.EOF {
					return n, err
				}
				return n, fmt.Errorf("array index [%d]: %w", i, err)
			}
		}
		return n, nil
	}

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
			m, err = ms.readMain(r, order, v, elementType, o, reflect.Value{}, -1)
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
			m, err = ms.readMain(r, order, newv, elementType, o, reflect.Value{}, -1)
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
func (ms *Marshaler) readStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
	if strc.CanInterface() {
		val := strc.Interface()
		if mr, ok := val.(MarshalerContextReader); ok {
			return mr.ReadBinaryWithMarshaler(ms, r, order)
		}
		if br, ok := val.(BinaryReader); ok {
			return br.ReadBinary(r, order)
		}
	}
	if strc.CanAddr() {
		addr := strc.Addr()
		if addr.CanInterface() {
			val := addr.Interface()
			if mr, ok := val.(MarshalerContextReader); ok {
				return mr.ReadBinaryWithMarshaler(ms, r, order)
			}
			if br, ok := val.(BinaryReader); ok {
				return br.ReadBinary(r, order)
			}
		}
	}

	if !safeMode {
		return ms.unsafeReadStruct(r, order, strc)
	}
	typ := strc.Type()
	meta, err := getStructMetadata(typ)
	if err != nil {
		return 0, err
	}
	// A struct-level byte order overrides the inherited order for this struct's
	// fields; per-field endian= still overrides it in turn.
	order = resolveByteOrder(order, meta.endian)

	firstElem := true
	wErr := func(i int, e error) error { // return a wrapped error
		if firstElem && (errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF)) {
			// If EOF occurs at the first non-ignoring field, then return a raw EOF
			return e
		}
		f := typ.Field(i)
		return &DecodeError{
			Offset: n,
			Field:  f.Name,
			Err:    e,
		}
	}

	for _, fMeta := range meta.fields {
		if fMeta.ignore {
			continue
		}
		if fMeta.unexported {
			continue
		}
		if fMeta.fieldErr != nil && !fMeta.hasTag {
			err = wErr(fMeta.index, fMeta.fieldErr)
			return
		}

		fieldVal := strc.Field(fMeta.index)
		fKind := typ.Field(fMeta.index).Type.Kind()

		if fMeta.omittable && fMeta.omittableExpr != "" {
			limit, errEval := evaluateTagValue(strc, fMeta.omittableExpr)
			if errEval == nil && n >= limit {
				break
			}
		}

		wasNilPtr := false
		if (fKind == reflect.Ptr || fKind == reflect.Interface) && fieldVal.IsNil() {
			wasNilPtr = true
		}

		v := fieldVal
		if (fKind == reflect.Ptr || fKind == reflect.Interface) && fMeta.codec == "" {
			// allocate pointers
			v, _ = dereferencePointer(v)
		}

		var naturalType eType
		var option typeOption
		if v.IsValid() {
			naturalType, option = getNaturalType(v)
		}

		if fMeta.hasTag {
			if naturalType == iInvalid && (fMeta.encodeType != Pad && fMeta.encodeType != Ignore) {
				if fMeta.fieldErr != nil {
					err = wErr(fMeta.index, fMeta.fieldErr)
				} else {
					err = wErr(fMeta.index, fmt.Errorf("the field %s is not encodable", fMeta.name))
				}
				return
			}
			if fMeta.encodeType != Any {
				naturalType = fMeta.encodeType
			}
			if fMeta.isArray {
				option.isArray = true
				if fMeta.arrayLenExpr != "" {
					option.arrayLen, err = evaluateTagValue(strc, fMeta.arrayLenExpr)
					if err != nil {
						err = wErr(fMeta.index, err)
						return
					}
					if option.arrayLen < 0 {
						err = wErr(fMeta.index, errNegativeSize)
						return
					}
				}
				if len(fMeta.arrayDimExprs) > 1 {
					option.dims = make([]int, len(fMeta.arrayDimExprs))
					for i, d := range fMeta.arrayDimExprs {
						if d == "" {
							if i == 0 {
								option.dims[i] = option.arrayLen
							}
							continue
						}
						option.dims[i], err = evaluateTagValue(strc, d)
						if err != nil {
							err = wErr(fMeta.index, err)
							return
						}
						if option.dims[i] < 0 {
							err = wErr(fMeta.index, errNegativeSize)
							return
						}
					}
					option.arrayLen = option.dims[0]
				}
			}
			if fMeta.bufLenExpr != "" {
				option.bufLen, err = evaluateTagValue(strc, fMeta.bufLenExpr)
				if err != nil {
					err = wErr(fMeta.index, err)
					return
				}
				if option.bufLen < 0 {
					err = wErr(fMeta.index, errNegativeSize)
					return
				}
			}
			if fMeta.encoding != "" {
				option.encoding = fMeta.encoding
			}
			if fMeta.endian != endianNone {
				option.endian = fMeta.endian
			}
			if fMeta.codec != "" {
				option.codec = fMeta.codec
			}
		}

		if fKind == reflect.Ptr || fKind == reflect.Interface {
			if fMeta.codec == "" {
				option.indirectCount = 0
			}
		}

		var m int
		m, err = ms.readMain(r, order, v, naturalType, option, strc, fMeta.index)
		if err != nil {
			if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) && m == 0 {
				if wasNilPtr {
					fieldVal.Set(reflect.Zero(fieldVal.Type()))
				}
				err = nil
				break
			}
			err = wErr(fMeta.index, err)
			return
		}
		if err = validateField(v, &fMeta); err != nil {
			err = wErr(fMeta.index, err)
			return
		}
		n += m
		firstElem = false
	}
	if err = ms.validateCustomValueofs(order, strc, meta, n, typ); err != nil {
		return
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
func (ms *Marshaler) readString(r io.Reader, order ByteOrder, v reflect.Value, encodeType eType, bufLen int, textEncoding string) (n int, err error) {
	if textEncoding == "" {
		textEncoding = ms.DefaultTextEncoding
	}

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
			buf, err = ms.DecodeText(buf, textEncoding)
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
			buf, err = ms.DecodeText(buf, textEncoding)
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
		buf, err = ms.DecodeText(buf, textEncoding)
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
	if bytesize > 1 && order == nil {
		err = errNoByteOrder
		return
	}
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
func (ms *Marshaler) readScalar(r io.Reader, order ByteOrder, v reflect.Value, k eType) (n int, err error) {
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

func validateField(v reflect.Value, fMeta *structFieldMetadata) error {
	if fMeta.hasConst {
		if err := validateConst(v, fMeta); err != nil {
			return err
		}
	}
	if !fMeta.hasRange && !fMeta.hasMatch {
		return nil
	}
	k := v.Kind()
	if k == reflect.Slice || k == reflect.Array {
		l := v.Len()
		for i := 0; i < l; i++ {
			if err := validateValue(v.Index(i), fMeta); err != nil {
				return err
			}
		}
		return nil
	}
	return validateValue(v, fMeta)
}

func validateValue(v reflect.Value, fMeta *structFieldMetadata) error {
	if fMeta.hasRange {
		var val float64
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val = float64(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val = float64(v.Uint())
		case reflect.Float32, reflect.Float64:
			val = v.Float()
		default:
			return fmt.Errorf("range validation not supported on type %s", v.Type().String())
		}
		if (fMeta.hasRangeMin && val < fMeta.rangeMin) || (fMeta.hasRangeMax && val > fMeta.rangeMax) {
			return fmt.Errorf("value %v is out of range [%g, %g]: %w", val, fMeta.rangeMin, fMeta.rangeMax, ErrValidationError)
		}
	}
	if fMeta.hasMatch {
		if v.Kind() != reflect.String {
			return fmt.Errorf("match validation not supported on type %s", v.Type().String())
		}
		if !fMeta.matchRegexp.MatchString(v.String()) {
			return fmt.Errorf("value %q does not match pattern %s: %w", v.String(), fMeta.matchPattern, ErrValidationError)
		}
	}
	return nil
}

// validateConst checks that a decoded field equals its const= value, returning
// an ErrValidationError on mismatch. Integers compare by value; byte-sequence
// fields compare bytes.
func validateConst(v reflect.Value, fMeta *structFieldMetadata) error {
	if fMeta.constIsBytes {
		got, ok := valueBytes(v)
		if !ok {
			return fmt.Errorf("const validation not supported on type %s", v.Type().String())
		}
		if !bytes.Equal(got, fMeta.constBytes) {
			return fmt.Errorf("const mismatch: got %#x, want %#x: %w", got, fMeta.constBytes, ErrValidationError)
		}
		return nil
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() != fMeta.constInt {
			return fmt.Errorf("const mismatch: got %d, want %d: %w", v.Int(), fMeta.constInt, ErrValidationError)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if v.Uint() != uint64(fMeta.constInt) {
			return fmt.Errorf("const mismatch: got %d, want %d: %w", v.Uint(), uint64(fMeta.constInt), ErrValidationError)
		}
	default:
		return fmt.Errorf("const validation not supported on type %s", v.Type().String())
	}
	return nil
}

// validateCustomValueofs runs after a struct is fully decoded: for every field
// with a custom valueof evaluator it recomputes the value (Decoding=true) from
// the decoded fields and checks it against what was read from the stream,
// returning a DecodeError wrapping ErrValidationError on mismatch. Running it as
// a post-decode pass (rather than inline) lets a checksum reference fields
// declared after it. The comparison is done on the encoded wire bytes so it is
// exact regardless of the target field's width, sign, or byte order. The decoded
// field is never overwritten (valueof stays emit-only, like bytelen/const).
func (ms *Marshaler) validateCustomValueofs(order ByteOrder, strc reflect.Value, meta *structMetadata, structEnd int, typ reflect.Type) error {
	for _, fMeta := range meta.fields {
		if fMeta.valueofCustomName == "" || fMeta.unexported || fMeta.ignore {
			continue
		}
		mkErr := func(e error) error {
			return &DecodeError{Offset: structEnd, Field: typ.Field(fMeta.index).Name, Err: e}
		}
		computed, err := ms.evalCustomValueof(order, strc, meta, fMeta, true)
		if err != nil {
			return mkErr(err)
		}
		wantBytes, err := ms.fieldEncodedBytes(order, strc, synthIntValue(strc.Field(fMeta.index), int(computed)), fMeta)
		if err != nil {
			return mkErr(err)
		}
		gotBytes, err := ms.fieldEncodedBytes(order, strc, strc.Field(fMeta.index), fMeta)
		if err != nil {
			return mkErr(err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			return mkErr(fmt.Errorf("valueof %s() mismatch: got %#x, want %#x: %w", fMeta.valueofCustomName, gotBytes, wantBytes, ErrValidationError))
		}
	}
	return nil
}

// valueBytes extracts the raw bytes of a string, byte slice, or byte array
// value for const comparison.
func valueBytes(v reflect.Value) ([]byte, bool) {
	switch v.Kind() {
	case reflect.String:
		return []byte(v.String()), true
	case reflect.Slice, reflect.Array:
		switch v.Type().Elem().Kind() {
		case reflect.Uint8:
			if v.Kind() == reflect.Slice {
				return v.Bytes(), true
			}
			b := make([]byte, v.Len())
			for i := range b {
				b[i] = byte(v.Index(i).Uint())
			}
			return b, true
		case reflect.Int8:
			b := make([]byte, v.Len())
			for i := range b {
				b[i] = byte(v.Index(i).Int())
			}
			return b, true
		}
	}
	return nil, false
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

// BinaryReader is implemented by types that can deserialize themselves from a stream.
type BinaryReader interface {
	ReadBinary(r io.Reader, order ByteOrder) (int, error)
}

// MarshalerContextReader is implemented by types that can deserialize themselves from a stream using a Marshaler context.
type MarshalerContextReader interface {
	ReadBinaryWithMarshaler(ms *Marshaler, r io.Reader, order ByteOrder) (int, error)
}
