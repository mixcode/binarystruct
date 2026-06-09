// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"bytes"
	stdencoding "encoding"
	"fmt"
	"io"
	"math"
	"reflect"

	"golang.org/x/text/encoding"
)

// Marshal encodes a go value into binary data and returns it as []byte. The byte
// order comes from the value's struct-level `endian=` declaration; for a value
// that declares none (e.g. a bare scalar), use NewMarshalerOrder(order).Marshal.
func Marshal(govalue interface{}) (encoded []byte, err error) {
	return NewMarshaler().Marshal(govalue)
}

// Write encodes a go value into a binary stream and writes it to w. See Marshal
// on where the byte order comes from.
func Write(w io.Writer, govalue interface{}) (n int, err error) {
	return NewMarshaler().Write(w, govalue)
}

// MarshalAs encodes a go value into binary data using the supplied tag and returns it as []byte.
func MarshalAs(govalue interface{}, tag string) (encoded []byte, err error) {
	return NewMarshaler().MarshalAs(govalue, tag)
}

// WriteAs encodes a go value into a binary stream using the supplied tag and writes it to w.
func WriteAs(w io.Writer, tag string, govalue interface{}) (n int, err error) {
	return NewMarshaler().WriteAs(w, tag, govalue)
}

// Append encodes a go value and appends the binary data to buf, returning the
// extended slice (mirroring encoding/binary.Append and the encoding.BinaryAppender idiom).
func Append(buf []byte, govalue interface{}) (out []byte, err error) {
	return NewMarshaler().Append(buf, govalue)
}

// Marshaler is a go-type to binary-type encoder/decoder with environmental values.
// The byte order is normally declared on the struct itself (a blank `_ struct{}`
// field tagged `binary:"endian=…"`, or inherited from an embedded struct). Order
// resolution, most specific first: a per-field `endian=` tag, then the struct's
// declaration, then the Marshaler's Order field (set via NewMarshalerOrder), and
// if none of those supply an order, encoding/decoding a multi-byte value fails
// with a clear error.
type Marshaler struct {
	Order               ByteOrder                    // fallback byte order for values that declare none; see NewMarshalerOrder
	TextEncoding        map[string]encoding.Encoding // map[encodingName]Encoding
	DefaultTextEncoding string                       // default text encoding name
	codecs              map[string]Codec             // registered custom codecs
	valueofs            map[string]ValueOfFunc       // registered custom valueof evaluators

	encoderCache map[string]*encoding.Encoder // cache of encoding.NewEncoder()
	decoderCache map[string]*encoding.Decoder // cache of encoding.NewDecoder()

	// scratch is a reusable 8-byte staging buffer for scalar reads/writes
	// (readU64/writeU64). Because it lives on the heap-allocated Marshaler, the
	// slice handed to io.Writer.Write / io.ReadFull no longer escapes to a fresh
	// per-call allocation. It is reused within a single (sequential) operation;
	// this is why a *Marshaler must not be shared across goroutines (see the
	// concurrency note on the package functions — the same rule already applies to
	// the lazily-populated encoder cache).
	scratch [8]byte
}

// NewMarshaler returns a Marshaler with no fallback byte order: values must
// declare their own order (struct-level `endian=` or a per-field override).
// Use NewMarshalerOrder to supply a fallback order for undeclared values.
func NewMarshaler() *Marshaler {
	return &Marshaler{}
}

// NewMarshalerOrder returns a Marshaler whose Order field supplies a fallback byte
// order for values that declare none. A struct-level or per-field `endian=` still
// takes precedence over it.
func NewMarshalerOrder(order ByteOrder) *Marshaler {
	return &Marshaler{Order: order}
}

// AddTextEncoding set a new text encoder to a Marshaler.
// Provided encodingName could be used in string tag's 'encoding' property, like `binary:"string,encoding=encodingName"`
func (ms *Marshaler) AddTextEncoding(encodingName string, enc encoding.Encoding) {
	if ms.TextEncoding == nil {
		ms.TextEncoding = make(map[string]encoding.Encoding)
	}
	ms.TextEncoding[encodingName] = enc
}

// RemoveTextEncoding removes an encoding from a Marshaler.
func (ms *Marshaler) RemoveTextEncoding(encodingName string) {
	if ms.TextEncoding != nil {
		delete(ms.TextEncoding, encodingName)
	}
	if ms.encoderCache != nil {
		delete(ms.encoderCache, encodingName)
	}
	if ms.decoderCache != nil {
		delete(ms.decoderCache, encodingName)
	}
}

// AddCodec registers a custom Codec with a Marshaler.
// Provided name could be used in struct field tags, like `binary:"...,codec=name"`
func (ms *Marshaler) AddCodec(name string, s Codec) {
	if ms.codecs == nil {
		ms.codecs = make(map[string]Codec)
	}
	ms.codecs[name] = s
}

// RemoveCodec removes a registered custom Codec.
func (ms *Marshaler) RemoveCodec(name string) {
	if ms.codecs != nil {
		delete(ms.codecs, name)
	}
}

// GetCodec returns a registered custom Codec by name, or nil if not found.
// This method is used by code generated by the codegen tool to look up codecs at runtime.
func (ms *Marshaler) GetCodec(name string) Codec {
	if ms.codecs != nil {
		if s, ok := ms.codecs[name]; ok {
			return s
		}
	}
	return nil
}

// AddValueOf registers a custom valueof evaluator with a Marshaler. The name may
// then be used in struct field tags, like `binary:"uint32,valueof=name(Field, ...)"`,
// to compute the field's serialized value (e.g. a CRC over other fields). The
// name must not collide with the built-ins bytelen or count.
func (ms *Marshaler) AddValueOf(name string, fn ValueOfFunc) {
	if ms.valueofs == nil {
		ms.valueofs = make(map[string]ValueOfFunc)
	}
	ms.valueofs[name] = fn
}

// RemoveValueOf removes a registered custom valueof evaluator.
func (ms *Marshaler) RemoveValueOf(name string) {
	if ms.valueofs != nil {
		delete(ms.valueofs, name)
	}
}

// GetValueOf returns a registered custom valueof evaluator by name, or nil if
// not found.
func (ms *Marshaler) GetValueOf(name string) ValueOfFunc {
	if ms.valueofs != nil {
		if fn, ok := ms.valueofs[name]; ok {
			return fn
		}
	}
	return nil
}

// Marshaler.Marshal() encodes a go value into binary data using the Marshaler's byte order.
func (ms *Marshaler) Marshal(govalue interface{}) (encoded []byte, err error) {
	var b bytes.Buffer
	_, err = ms.Write(&b, govalue)
	return b.Bytes(), err
}

// Marshaler.MarshalAs() encodes a go value using the supplied tag and the Marshaler's byte order.
func (ms *Marshaler) MarshalAs(govalue interface{}, tag string) (encoded []byte, err error) {
	var b bytes.Buffer
	_, err = ms.WriteAs(&b, tag, govalue)
	return b.Bytes(), err
}

// Marshaler.Write() encodes a go value into a binary stream. The byte order comes
// from the value's declaration, falling back to the Marshaler's Order field.
func (ms *Marshaler) Write(w io.Writer, data interface{}) (n int, err error) {
	return ms.writeValue(w, ms.Order, reflect.ValueOf(data))
}

// Marshaler.WriteAs() encodes a go value into a binary stream using the supplied tag.
func (ms *Marshaler) WriteAs(w io.Writer, tag string, data interface{}) (n int, err error) {
	order := ms.Order
	v := reflect.ValueOf(data)
	t := v.Type()
	k := t.Kind()
	for k == reflect.Ptr || k == reflect.Interface {
		v = reflect.Indirect(v)
		t = v.Type()
		k = t.Kind()
	}

	var fieldErr error
	switch k {
	case reflect.Invalid:
		fieldErr = fmt.Errorf("invalid data type")
	case reflect.Complex64, reflect.Complex128:
		fieldErr = fmt.Errorf("complex type not supported")
	case reflect.UnsafePointer:
		fieldErr = fmt.Errorf("pointer type not supported")
	case reflect.Chan, reflect.Func, reflect.Map:
		fieldErr = fmt.Errorf("unsupported type: %v", k)
	}

	naturalType, naturalOption := getNaturalType(v)
	encodeType, option, err := parseTagString(tag, reflect.Value{}, naturalType, naturalOption, fieldErr)
	if err != nil {
		return 0, err
	}

	return ms.writeMain(w, order, v, encodeType, option, reflect.Value{}, -1)
}

// Marshaler.Append() encodes a go value and appends the binary data to buf using
// the Marshaler's byte order, returning the extended slice. A nil buf is allowed.
func (ms *Marshaler) Append(buf []byte, data interface{}) (out []byte, err error) {
	b := bytes.NewBuffer(buf)
	_, err = ms.Write(b, data)
	return b.Bytes(), err
}

// write a reflect.Value
func (ms *Marshaler) writeValue(w io.Writer, order ByteOrder, v reflect.Value) (n int, err error) {
	t := v.Type()
	k := t.Kind()
	for k == reflect.Ptr || k == reflect.Interface {
		v = reflect.Indirect(v)
		t = v.Type()
		k = t.Kind()
	}
	encodeType, option := getNaturalType(v)

	return ms.writeMain(w, order, v, encodeType, option, reflect.Value{}, -1)
}

// write a value as given type
func (ms *Marshaler) writeMain(w io.Writer, order ByteOrder, v reflect.Value, encodeType eType, option typeOption, parentStruct reflect.Value, fieldIndex int) (n int, err error) {

	order = resolveByteOrder(order, option.endian)

	if option.codec != "" {
		codec, ok := ms.codecs[option.codec]
		if !ok {
			return 0, fmt.Errorf("unknown codec: %s", option.codec)
		}
		return codec.Encode(w, v.Interface(), parentStruct, fieldIndex, order)
	}

	if encodeType == Any {
		var naturalOption typeOption
		encodeType, naturalOption = getNaturalType(v)
		option.indirectCount += naturalOption.indirectCount
		// Adopt the natural array length when the tag left it unknown (e.g. an
		// untagged nested Go array): without this the element would be silently
		// skipped (arrayLen 0). 1-D fields already carry their length here.
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
		// write the array. arrayLen==0 means an empty 1-D array (write nothing);
		// a multidimensional tag still recurses (its outer length comes from the
		// value when the outer dimension is implicit).
		if option.arrayLen == 0 && len(option.dims) <= 1 {
			return
		}
		return ms.writeArray(w, order, v, encodeType, option)
	}

	// based on individual type
	switch encodeType {

	case iStruct:
		return ms.writeStruct(w, order, v)

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
		return ms.writeScalar(w, order, v, encodeType)

	case structKind:
		return ms.writeStruct(w, order, v)

	case stringKind:
		return ms.writeString(w, order, v, encodeType, option.bufLen, option.encoding)
	}

	err = fmt.Errorf("unknown type %s", encodeType)
	return
}

// write an array
func (ms *Marshaler) writeArray(w io.Writer, order ByteOrder, array reflect.Value, elementType eType, option typeOption) (n int, err error) {

	// Multidimensional tag (e.g. [4][2]int8): each outer element is itself an
	// array of the remaining dimensions. Recurse through writeMain so any leaf
	// type works; the innermost dimension falls through to the 1-D path below.
	if len(option.dims) > 1 {
		if array.Kind() != reflect.Array && array.Kind() != reflect.Slice {
			return 0, fmt.Errorf("multidimensional binary tag on non-array value of kind %s", array.Kind())
		}
		actualLen := array.Len()
		desiredLen := option.dims[0]
		if desiredLen <= 0 {
			desiredLen = actualLen
		}
		if actualLen > desiredLen {
			return 0, fmt.Errorf("array too large to fit: len %d, size %d", desiredLen, actualLen)
		}
		child := option
		child.dims = option.dims[1:]
		child.arrayLen = child.dims[0]
		child.isArray = true
		elemType := array.Type().Elem()
		var m int
		for i := 0; i < desiredLen; i++ {
			e := reflect.Zero(elemType) // pad missing outer elements with zero sub-arrays
			if i < actualLen {
				e = array.Index(i)
			}
			m, err = ms.writeMain(w, order, e, elementType, child, reflect.Value{}, -1)
			n += m
			if err != nil {
				return n, fmt.Errorf("array index [%d]: %w", i, err)
			}
		}
		return n, nil
	}

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
			m, err = ms.writeValue(w, order, e)
			if err != nil {
				err = wErr(i, err)
				return
			}
		} else {
			var o typeOption
			o.bufLen = option.bufLen     // option may contain inheritable values
			o.encoding = option.encoding // option may contain inheritable values
			m, err = ms.writeMain(w, order, e, elementType, o, reflect.Value{}, -1)
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
func (ms *Marshaler) writeStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	if strc.CanInterface() {
		val := strc.Interface()
		if mw, ok := val.(MarshalerContextWriter); ok {
			return mw.WriteBinaryWithMarshaler(ms, w, order)
		}
		if bw, ok := val.(BinaryWriter); ok {
			return bw.WriteBinary(w, order)
		}
		if bm, ok := val.(stdencoding.BinaryMarshaler); ok {
			var blob []byte
			blob, err = bm.MarshalBinary()
			if err != nil {
				return 0, err
			}
			return w.Write(blob)
		}
	}
	if strc.CanAddr() {
		addr := strc.Addr()
		if addr.CanInterface() {
			val := addr.Interface()
			if mw, ok := val.(MarshalerContextWriter); ok {
				return mw.WriteBinaryWithMarshaler(ms, w, order)
			}
			if bw, ok := val.(BinaryWriter); ok {
				return bw.WriteBinary(w, order)
			}
			if bm, ok := val.(stdencoding.BinaryMarshaler); ok {
				var blob []byte
				blob, err = bm.MarshalBinary()
				if err != nil {
					return 0, err
				}
				return w.Write(blob)
			}
		}
	}

	if !safeMode {
		return ms.unsafeWriteStruct(w, order, strc)
	}
	typ := strc.Type()
	meta, err := getStructMetadata(typ)
	if err != nil {
		return 0, err
	}
	// A struct-level byte order (declared via a `_` sentinel or inherited from an
	// embedded struct) overrides the inherited order for this struct's fields;
	// per-field endian= still overrides it in turn.
	order = resolveByteOrder(order, meta.endian)
	wErr := func(i int, e error) error {
		f := typ.Field(i)
		return fmt.Errorf("field <%s>: %w", f.Name, e)
	}
	writeEval := ms.encodeExprEval(order, strc, meta)
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

		if fMeta.omittable {
			if fMeta.omittableExpr != "" {
				limit, errEval := evaluateTagValue(strc, fMeta.omittableExpr)
				if errEval == nil && n >= limit {
					break
				}
			}
			fKind := typ.Field(fMeta.index).Type.Kind()
			if (fKind == reflect.Ptr || fKind == reflect.Interface) && fieldVal.IsNil() {
				break
			}
		}

		naturalType, option, errF := ms.resolveFieldEncoding(fieldVal, fMeta, writeEval)
		if errF != nil {
			err = wErr(fMeta.index, errF)
			return
		}

		// valueof: replace the field's value with one computed from other
		// fields (emit-only; the source struct is never modified).
		if fMeta.valueofExpr != "" {
			if fMeta.valueofCustomName != "" {
				computed, errV := ms.evalCustomValueof(order, strc, meta, fMeta, false)
				if errV != nil {
					err = wErr(fMeta.index, errV)
					return
				}
				fieldVal = synthIntValue(fieldVal, int(computed))
			} else {
				computed, errV := ms.evalValueof(order, strc, meta, fMeta.valueofExpr)
				if errV != nil {
					err = wErr(fMeta.index, errV)
					return
				}
				fieldVal = synthIntValue(fieldVal, computed)
			}
		}

		// const: emit the fixed value, ignoring the struct field (emit-only).
		if fMeta.hasConst {
			if fMeta.constIsBytes {
				fieldVal = synthBytesValue(fieldVal, fMeta.constBytes)
			} else {
				fieldVal = synthIntValue(fieldVal, int(fMeta.constInt))
			}
		}

		var m int
		m, err = ms.writeMain(w, order, fieldVal, naturalType, option, strc, fMeta.index)
		if err != nil {
			err = wErr(fMeta.index, err)
			return
		}
		n += m
	}
	return
}

// resolveFieldEncoding determines the binary encode type and options for a
// struct field from its metadata. Decode-side size expressions are evaluated
// through the supplied evalExpr strategy (which differs between the write path
// and bytelen() measurement). Errors are returned unwrapped for the caller to
// annotate. It does not apply valueof, which overrides the field's value rather
// than its type. Shared by the safe and unsafe encode paths and by measurement.
func (ms *Marshaler) resolveFieldEncoding(fieldVal reflect.Value, fMeta structFieldMetadata, evalExpr func(string) (int, error)) (naturalType eType, option typeOption, err error) {
	if fieldVal.IsValid() {
		naturalType, option = getNaturalType(fieldVal)
	}
	if !fMeta.hasTag {
		return
	}
	if naturalType == iInvalid && fMeta.encodeType != Pad && fMeta.encodeType != Ignore {
		if fMeta.fieldErr != nil {
			return naturalType, option, fMeta.fieldErr
		}
		return naturalType, option, fmt.Errorf("the field %s is not encodable", fMeta.name)
	}
	if fMeta.encodeType != Any {
		naturalType = fMeta.encodeType
	}
	if fMeta.isArray {
		option.isArray = true
		switch {
		case fMeta.arrayLenConst && len(fMeta.arrayDimExprs) <= 1:
			// Common 1-D constant length: use the value pre-resolved at metadata
			// time instead of re-evaluating the expression every operation.
			option.arrayLen = fMeta.option.arrayLen
		case len(fMeta.arrayDimExprs) > 0:
			// Resolve every dimension of a (possibly multidimensional) array tag. An
			// empty dimension expression keeps the length seeded from the value itself
			// (getNaturalType above), e.g. `[]byte` or an implicit outer `[][3]int8`.
			option.dims = make([]int, len(fMeta.arrayDimExprs))
			for i, d := range fMeta.arrayDimExprs {
				if d == "" {
					if i == 0 {
						option.dims[i] = option.arrayLen // natural outer length
					}
					continue
				}
				dv, e := evalExpr(d)
				if e != nil {
					return naturalType, option, e
				}
				if dv < 0 {
					return naturalType, option, errNegativeSize
				}
				option.dims[i] = dv
			}
			option.arrayLen = option.dims[0]
		}
	}
	if fMeta.bufLenConst {
		// Pre-resolved constant buffer size (e.g. string(16)).
		option.bufLen = fMeta.option.bufLen
	} else if fMeta.bufLenExpr != "" {
		option.bufLen, err = evalExpr(fMeta.bufLenExpr)
		if err != nil {
			return
		}
		if option.bufLen < 0 {
			return naturalType, option, errNegativeSize
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
	return
}

// encodeExprEval returns the write-path size-expression evaluator: it resolves
// referenced valueof fields to their computed values (not their ignored Go
// field values), so a target's size expression (e.g. [NameLen]byte) agrees with
// the valueof-computed length actually written to the stream.
func (ms *Marshaler) encodeExprEval(order ByteOrder, strc reflect.Value, meta *structMetadata) func(string) (int, error) {
	return func(expr string) (int, error) {
		return ms.evalEncodeExpr(order, strc, meta, expr)
	}
}

// evalEncodeExpr evaluates a size expression at ENCODE time, resolving any
// referenced valueof field to its computed value. Functions are not permitted
// in size expressions.
func (ms *Marshaler) evalEncodeExpr(order ByteOrder, strc reflect.Value, meta *structMetadata, expr string) (int, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	base := fieldValueResolver(strc)
	p := &tagParser{
		tokens: tokens,
		strc:   strc,
		resolveIdent: func(name string) (int, error) {
			if fm, ok := meta.fieldByName(name); ok && fm.valueofExpr != "" {
				return ms.evalValueof(order, strc, meta, fm.valueofExpr)
			}
			return base(name)
		},
	}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.peek().typ != tokEOF {
		return 0, fmt.Errorf("unexpected token at end of expression: %s", p.peek().val)
	}
	return v, nil
}

// evalValueof evaluates a valueof expression at encode time, resolving
// bytelen()/count() against the live struct value.
func (ms *Marshaler) evalValueof(order ByteOrder, strc reflect.Value, meta *structMetadata, expr string) (int, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	p := &tagParser{
		tokens:       tokens,
		strc:         strc,
		resolveIdent: fieldValueResolver(strc),
		callFunc: func(fn string, args []string) (int, error) {
			return ms.evalValueofFunc(order, strc, meta, fn, args)
		},
	}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.peek().typ != tokEOF {
		return 0, fmt.Errorf("unexpected token at end of valueof expression: %s", p.peek().val)
	}
	return v, nil
}

// evalValueofFunc computes the built-in bytelen(field) or count(field). Only
// these two single-argument built-ins flow through here; custom multi-argument
// evaluators are dispatched separately (see evalCustomValueof).
func (ms *Marshaler) evalValueofFunc(order ByteOrder, strc reflect.Value, meta *structMetadata, fn string, args []string) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s() takes exactly one field-name argument", fn)
	}
	arg := args[0]
	fMeta, ok := meta.fieldByName(arg)
	if !ok {
		return 0, fmt.Errorf("valueof: no field named %s", arg)
	}
	fieldVal := strc.Field(fMeta.index)

	switch fn {
	case "count":
		v := derefValue(fieldVal)
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			return v.Len(), nil
		default:
			return 0, fmt.Errorf("count(%s): field is not a slice or array (use bytelen for a string's byte length)", arg)
		}
	case "bytelen":
		return ms.fieldEncodedSize(order, strc, fieldVal, fMeta)
	default:
		return 0, fmt.Errorf("unknown function %s", fn)
	}
}

// fieldEncodedSize returns the number of bytes the field would occupy when
// encoded.
func (ms *Marshaler) fieldEncodedSize(order ByteOrder, strc, fieldVal reflect.Value, fMeta structFieldMetadata) (int, error) {
	b, err := ms.fieldEncodedBytes(order, strc, fieldVal, fMeta)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// fieldEncodedBytes returns the exact bytes the field would occupy when encoded.
// For a raw byte region (byte slice/array or an unencoded string at its natural
// length) it returns the field's own bytes directly — those encode unchanged, so a
// second pass would be pure overhead. Every other shape is measured by encoding
// into a scratch buffer with the same logic as the real write, so it stays exact
// for text-encoded strings, length-prefixed strings, and nested structs (the bytes
// a checksum must operate on).
func (ms *Marshaler) fieldEncodedBytes(order ByteOrder, strc, fieldVal reflect.Value, fMeta structFieldMetadata) ([]byte, error) {
	// Measurement strategy: honor constant sizes (e.g. string(16), [4]byte) but
	// use the value's own length for field-referencing expressions. This makes
	// bytelen() measure the field's actual content and avoids infinite recursion
	// when a length field's valueof references the very slice/string it sizes
	// (the canonical [NameLen]byte / valueof=bytelen(Name) pair).
	naturalEval := func(expr string) (int, error) {
		refs, _, e := exprReferences(expr)
		if e != nil {
			return 0, e
		}
		if len(refs) == 0 {
			return evaluateTagValue(reflect.Value{}, expr) // constant size
		}
		v := derefValue(fieldVal)
		switch v.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			// writeMain treats arrayLen==0 as "no elements", so the natural
			// element count must be supplied explicitly.
			return v.Len(), nil
		case reflect.String:
			// bufLen==0 makes writeString emit the full encoded form with no
			// padding, which is the exact encoded byte length (correct even for
			// multibyte text encodings like Shift-JIS).
			return 0, nil
		}
		return 0, nil
	}
	naturalType, option, err := ms.resolveFieldEncoding(fieldVal, fMeta, naturalEval)
	if err != nil {
		return nil, err
	}
	// Fast path: a raw byte region encodes to exactly its in-memory bytes, so hand
	// them back instead of a second per-element reflective encode (the hot path for
	// checksums and bytelen() over []byte). resolveFieldEncoding ran first, so the
	// [NameLen]byte / valueof=bytelen(Name) recursion guard in naturalEval still
	// applies. See TODO "Runtime custom-evaluator perf".
	if b, ok := rawByteRegionBytes(fieldVal, naturalType, option); ok {
		return b, nil
	}
	var buf bytes.Buffer
	if _, err := ms.writeMain(&buf, order, fieldVal, naturalType, option, strc, fMeta.index); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rawByteRegionBytes returns a field's encoded bytes directly when its encoded
// form is byte-identical to its in-memory bytes: a byte slice or [N]byte array
// written at its natural length, or an unencoded raw string. Such fields carry no
// byte-order, padding, text-encoding, or codec transform, so the scratch-buffer
// re-encode in fieldEncodedBytes is pure overhead. ok is false when the slow path
// must run — a constant fixed length that pads/truncates, a text encoding, a
// codec, a length-prefixed/terminated string, or a non-uint8 element type.
//
// For a byte slice the returned slice aliases the field's backing array (the
// zero-copy intent); evaluators read it (e.g. to hash), they must not mutate it.
func rawByteRegionBytes(fieldVal reflect.Value, naturalType eType, option typeOption) ([]byte, bool) {
	if option.encoding != "" || option.codec != "" {
		return nil, false
	}
	v := derefValue(fieldVal)
	switch v.Kind() {
	case reflect.Slice:
		if naturalType != Byte && naturalType != Uint8 && naturalType != Int8 {
			return nil, false
		}
		if v.Type().Elem().Kind() != reflect.Uint8 {
			return nil, false // e.g. []int8: reflect.Value.Bytes would panic
		}
		// Only when the write emits the slice as-is; a constant fixed length that
		// differs from the value pads/truncates, so defer to the slow path.
		if option.arrayLen != 0 && option.arrayLen != v.Len() {
			return nil, false
		}
		return v.Bytes(), true
	case reflect.Array:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			return nil, false
		}
		if option.arrayLen != 0 && option.arrayLen != v.Len() {
			return nil, false
		}
		if v.CanAddr() {
			return v.Slice(0, v.Len()).Bytes(), true
		}
		out := make([]byte, v.Len())
		for i := range out {
			out[i] = byte(v.Index(i).Uint())
		}
		return out, true
	case reflect.String:
		if naturalType != String || option.bufLen != 0 {
			return nil, false
		}
		return []byte(v.String()), true
	}
	return nil, false
}

// evalCustomValueof computes a custom valueof field's value: it materializes the
// encoded bytes (and Go value) of each referenced field, then invokes the
// evaluator registered on the Marshaler. The same evaluator runs on encode
// (decoding=false) to produce the value to write and on decode (decoding=true)
// to recompute it for validation.
func (ms *Marshaler) evalCustomValueof(order ByteOrder, strc reflect.Value, meta *structMetadata, fMeta structFieldMetadata, decoding bool) (uint64, error) {
	fn := ms.GetValueOf(fMeta.valueofCustomName)
	if fn == nil {
		return 0, fmt.Errorf("valueof: no evaluator named %q registered on the Marshaler (use AddValueOf)", fMeta.valueofCustomName)
	}
	args := make([]ValueOfArg, 0, len(fMeta.valueofCustomArgs))
	for _, name := range fMeta.valueofCustomArgs {
		af, ok := meta.fieldByName(name)
		if !ok {
			return 0, fmt.Errorf("valueof %s(): no field named %s", fMeta.valueofCustomName, name)
		}
		fieldVal := strc.Field(af.index)
		b, err := ms.fieldEncodedBytes(order, strc, fieldVal, af)
		if err != nil {
			return 0, fmt.Errorf("valueof %s(): encoding field %s: %w", fMeta.valueofCustomName, name, err)
		}
		var val interface{}
		if fieldVal.CanInterface() {
			val = fieldVal.Interface()
		}
		args = append(args, ValueOfArg{Name: name, Bytes: b, Value: val})
	}
	var structPtr interface{}
	if strc.CanAddr() {
		structPtr = strc.Addr().Interface()
	} else {
		cp := reflect.New(strc.Type())
		cp.Elem().Set(strc)
		structPtr = cp.Interface()
	}
	return fn(ValueOfContext{
		Struct:   structPtr,
		Target:   fMeta.name,
		Args:     args,
		Decoding: decoding,
	})
}

// synthIntValue builds a reflect.Value holding v, typed like orig when orig is
// an integer kind, otherwise a plain int (which the scalar writer converts to
// the field's encode type).
func synthIntValue(orig reflect.Value, v int) reflect.Value {
	if orig.IsValid() {
		switch orig.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			nv := reflect.New(orig.Type()).Elem()
			nv.SetInt(int64(v))
			return nv
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			nv := reflect.New(orig.Type()).Elem()
			nv.SetUint(uint64(v))
			return nv
		}
	}
	return reflect.ValueOf(v)
}

// synthBytesValue returns a value of orig's type holding the given bytes, used
// to emit a byte-sequence const without mutating the source field. orig may be
// a string, a byte slice, or a fixed byte array; len(b) is guaranteed to match
// a fixed array's length by const metadata validation.
func synthBytesValue(orig reflect.Value, b []byte) reflect.Value {
	if orig.IsValid() {
		switch orig.Kind() {
		case reflect.String:
			nv := reflect.New(orig.Type()).Elem()
			nv.SetString(string(b))
			return nv
		case reflect.Slice:
			nv := reflect.MakeSlice(orig.Type(), len(b), len(b))
			reflect.Copy(nv, reflect.ValueOf(b))
			return nv
		case reflect.Array:
			nv := reflect.New(orig.Type()).Elem()
			reflect.Copy(nv, reflect.ValueOf(b))
			return nv
		}
	}
	return reflect.ValueOf(b)
}

// derefValue follows pointers and interfaces to the underlying value.
func derefValue(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			break
		}
		v = v.Elem()
	}
	return v
}

// EncodeText encodes a string with installed encoding.
func (ms *Marshaler) EncodeText(utf8 []byte, textEncoding string) (encoded []byte, err error) {
	if textEncoding == "" {
		encoded = utf8
		return
	}
	var ec *encoding.Encoder
	if ms.encoderCache != nil {
		ec = ms.encoderCache[textEncoding]
	}
	if ec == nil {
		if ms.TextEncoding != nil {
			ec = ms.TextEncoding[textEncoding].NewEncoder()
		}
		if ec == nil {
			err = fmt.Errorf("unknown text encoding %s", textEncoding)
			return
		}
		if ms.encoderCache == nil {
			ms.encoderCache = make(map[string]*encoding.Encoder)
		}
		ms.encoderCache[textEncoding] = ec
	}
	return ec.Bytes(utf8)
}

// DecodeText decodes a string with installed encoding.
func (ms *Marshaler) DecodeText(encoded []byte, textEncoding string) (utf8 []byte, err error) {
	if textEncoding == "" {
		utf8 = encoded
		return
	}
	var dc *encoding.Decoder
	if ms.decoderCache != nil {
		dc = ms.decoderCache[textEncoding]
	}
	if dc == nil {
		if ms.TextEncoding != nil {
			dc = ms.TextEncoding[textEncoding].NewDecoder()
		}
		if dc == nil {
			err = fmt.Errorf("unknown text encoding %s", textEncoding)
			return
		}
		if ms.decoderCache == nil {
			ms.decoderCache = make(map[string]*encoding.Decoder)
		}
		ms.decoderCache[textEncoding] = dc
	}
	return dc.Bytes(encoded)
}

// write string types
func (ms *Marshaler) writeString(w io.Writer, order ByteOrder, v reflect.Value, encodeType eType, bufLen int, textEncoding string) (n int, err error) {
	s := v.String()
	stringBytes := []byte(s)

	var m int

	// process text encoding
	if textEncoding == "" {
		textEncoding = ms.DefaultTextEncoding
	}
	if textEncoding != "" {
		stringBytes, err = ms.EncodeText(stringBytes, textEncoding)
		if err != nil {
			return
		}
	}

	strlen := len(stringBytes)
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
		m, err = ms.writeU64(w, order, uint64(strlen), headersz)
		n += m
		if err != nil {
			return
		}
	}

	// write string bytes
	m, err = w.Write(stringBytes)
	n += m
	if err != nil {
		return
	}

	// write terminating zero
	switch encodeType {
	case Zstring:
		w.Write(terminatingZeros[:1])
		n += 1
		m += 1
	case Z16string:
		w.Write(terminatingZeros[:2])
		n += 2
		m += 2
		//case Z32string:
		//	w.Write(terminatingZeros[:4])
	}

	if m < bufLen {
		// fill the leftovers
		m, err = zeroFill(w, bufLen-m)
		n += m
		if err != nil {
			return
		}
		return
	}

	return
}

// write a scalar value
func (ms *Marshaler) writeScalar(w io.Writer, order ByteOrder, v reflect.Value, k eType) (n int, err error) {
	enc := encodeFunc(v.Type(), k)
	if enc == nil {
		err = ErrInvalidType
		return
	}
	u64, sz, err := enc(v)
	if err != nil {
		return
	}
	return ms.writeU64(w, order, u64, sz)
}

// write bytes according to the byte order. The staging buffer is the Marshaler's
// reusable scratch (not a fresh per-call array), so b does not escape to a new
// allocation through w.Write — the single biggest per-scalar alloc otherwise.
func (ms *Marshaler) writeU64(w io.Writer, order ByteOrder, u64 uint64, bytesize int) (n int, err error) {
	b := ms.scratch[:bytesize]
	if bytesize > 1 && order == nil {
		return 0, errNoByteOrder
	}
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

var (
	terminatingZeros = []byte{0, 0, 0, 0}
)

// BinaryWriter is implemented by types that can serialize themselves to a stream.
type BinaryWriter interface {
	WriteBinary(w io.Writer, order ByteOrder) (int, error)
}

// MarshalerContextWriter is implemented by types that can serialize themselves to a stream using a Marshaler context.
type MarshalerContextWriter interface {
	WriteBinaryWithMarshaler(ms *Marshaler, w io.Writer, order ByteOrder) (int, error)
}
