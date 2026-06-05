// Copyright 2026 github.com/mixcode

//go:build !safe_binarystruct

package binarystruct

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"unsafe"
)

var hostEndian binary.ByteOrder

func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)
	if buf[0] == 0xAB {
		hostEndian = binary.BigEndian
	} else {
		hostEndian = binary.LittleEndian
	}
}

func getITypeFromRKind(k reflect.Kind) eType {
	switch k {
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
	case reflect.Bool:
		return Uint8
	case reflect.Int:
		return Int64
	case reflect.Uint:
		return Uint64
	case reflect.String:
		return String
	case reflect.Struct:
		return iStruct
	case reflect.Array, reflect.Slice:
		return iArray
	case reflect.Ptr, reflect.Interface:
		return Any
	}
	return iInvalid
}

func (ms *Marshaler) unsafeWriteStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	typ := strc.Type()
	meta, err := getStructMetadata(typ)
	if err != nil {
		return 0, err
	}
	// A struct-level byte order overrides the inherited order for this struct's
	// fields; per-field endian= still overrides it in turn.
	order = resolveByteOrder(order, meta.endian)

	var base unsafe.Pointer
	if strc.CanAddr() {
		base = unsafe.Pointer(strc.Addr().Pointer())
	} else {
		copyVal := reflect.New(strc.Type()).Elem()
		copyVal.Set(strc)
		base = unsafe.Pointer(copyVal.Addr().Pointer())
	}
	wErr := func(i int, e error) error {
		f := typ.Field(i)
		return fmt.Errorf("field <%s>: %w", f.Name, e)
	}
	// Write-path size-expression evaluator: resolves referenced valueof fields
	// to their computed values rather than their ignored Go field values.
	writeEval := ms.encodeExprEval(order, strc, meta)

	for _, fMeta := range meta.fields {
		if fMeta.ignore || fMeta.unexported {
			continue
		}

		if fMeta.fieldErr != nil && !fMeta.hasTag {
			err = wErr(fMeta.index, fMeta.fieldErr)
			return
		}

		// check omittable expr
		if fMeta.omittable && fMeta.omittableExpr != "" {
			limit, errEval := evaluateTagValue(strc, fMeta.omittableExpr)
			if errEval == nil && n >= limit {
				break
			}
		}

		// valueof: integer field whose serialized value is computed from other
		// fields (emit-only). Route through the reflection writer with the
		// computed value instead of the stale in-memory value.
		if fMeta.valueofExpr != "" {
			computed, errV := ms.evalValueof(order, strc, meta, fMeta.valueofExpr)
			if errV != nil {
				err = wErr(fMeta.index, errV)
				return
			}
			syn := synthIntValue(strc.Field(fMeta.index), computed)
			naturalType, option, errF := ms.resolveFieldEncoding(syn, fMeta, writeEval)
			if errF != nil {
				err = wErr(fMeta.index, errF)
				return
			}
			var m int
			m, err = ms.writeMain(w, order, syn, naturalType, option, strc, fMeta.index)
			if err != nil {
				err = wErr(fMeta.index, err)
				return
			}
			n += m
			continue
		}

		// const: emit a fixed value (emit-only), routed through the reflection
		// writer with the constant, like valueof.
		if fMeta.hasConst {
			var syn reflect.Value
			if fMeta.constIsBytes {
				syn = synthBytesValue(strc.Field(fMeta.index), fMeta.constBytes)
			} else {
				syn = synthIntValue(strc.Field(fMeta.index), int(fMeta.constInt))
			}
			naturalType, option, errF := ms.resolveFieldEncoding(syn, fMeta, writeEval)
			if errF != nil {
				err = wErr(fMeta.index, errF)
				return
			}
			var m int
			m, err = ms.writeMain(w, order, syn, naturalType, option, strc, fMeta.index)
			if err != nil {
				err = wErr(fMeta.index, err)
				return
			}
			n += m
			continue
		}

		fieldPtr := unsafe.Add(base, fMeta.offset)
		currType := typ.Field(fMeta.index).Type
		// dereference pointers/interfaces
		currPtr := fieldPtr
		isNil := false
		indirectCount := 0
		for currType.Kind() == reflect.Ptr || currType.Kind() == reflect.Interface {
			indirectCount++
			if currType.Kind() == reflect.Ptr {
				valPtr := *(*unsafe.Pointer)(currPtr)
				if valPtr == nil {
					isNil = true
					break
				}
				currPtr = valPtr
				currType = currType.Elem()
			} else {
				// interface fallback
				break
			}
		}

		if fMeta.omittable && isNil {
			break
		}

		// If it's interface, nil, or has custom codec, fall back to reflection
		if typ.Field(fMeta.index).Type.Kind() == reflect.Interface || fMeta.codec != "" || isNil {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType, option := getNaturalType(fieldVal)
			if fMeta.hasTag {
				if fMeta.encodeType != Any {
					naturalType = fMeta.encodeType
				}
				if fMeta.isArray {
					option.isArray = true
					if fMeta.arrayLenExpr != "" {
						option.arrayLen, err = writeEval(fMeta.arrayLenExpr)
						if err != nil {
							return n, wErr(fMeta.index, err)
						}
					}
				}
				if fMeta.bufLenExpr != "" {
					option.bufLen, err = writeEval(fMeta.bufLenExpr)
					if err != nil {
						return n, wErr(fMeta.index, err)
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
			m, err = ms.writeMain(w, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			continue
		}

		fieldOrder := resolveByteOrder(order, fMeta.endian)

		// Check if it's a nested struct
		fieldValType := typ.Field(fMeta.index).Type
		for i := 0; i < indirectCount; i++ {
			fieldValType = fieldValType.Elem()
		}

		if fieldValType.Kind() == reflect.Struct {
			structVal := reflect.NewAt(fieldValType, currPtr).Elem()
			var m int
			m, err = ms.unsafeWriteStruct(w, fieldOrder, structVal)
			if err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			continue
		}

		// Handle array/slice fields
		if fMeta.isArray || fieldValType.Kind() == reflect.Slice || fieldValType.Kind() == reflect.Array {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType := fMeta.naturalType
			option := fMeta.option
			if fMeta.arrayLenExpr != "" {
				option.arrayLen, err = writeEval(fMeta.arrayLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			} else if fieldValType.Kind() == reflect.Slice {
				sh := (*sliceHeader)(currPtr)
				option.arrayLen = sh.Len
			}
			if fMeta.bufLenExpr != "" {
				option.bufLen, err = writeEval(fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			var ok bool
			if fieldValType.Kind() == reflect.Slice || fieldValType.Kind() == reflect.Array {
				m, ok, err = ms.unsafeWriteSlice(w, fieldOrder, currPtr, fieldValType.Kind() == reflect.Slice, option.arrayLen, fMeta.naturalType, fieldValType.Elem())
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			if ok {
				n += m
				continue
			}
			m, err = ms.writeMain(w, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			continue
		}

		// Handle string fields
		if fieldValType.Kind() == reflect.String {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType := fMeta.naturalType
			option := fMeta.option
			if fMeta.bufLenExpr != "" {
				option.bufLen, err = writeEval(fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			m, err = ms.writeMain(w, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			continue
		}

		// Handle padding field
		if fMeta.encodeType == Pad {
			l := 1
			if fMeta.bufLenExpr != "" {
				l, err = writeEval(fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			var m int
			m, err = zeroFill(w, l)
			if err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			continue
		}

		// Handle basic scalar fields using unsafe
		var m int
		m, err = ms.unsafeWriteScalar(w, fieldOrder, currPtr, fMeta.encodeType, fieldValType.Kind())
		if err != nil {
			return n, wErr(fMeta.index, err)
		}
		n += m
	}
	return
}

func (ms *Marshaler) unsafeWriteScalar(w io.Writer, order ByteOrder, ptr unsafe.Pointer, encodeType eType, goKind reflect.Kind) (n int, err error) {
	k := encodeType
	if k == Any || k == iInvalid {
		k = getITypeFromRKind(goKind)
	}

	sz := k.ByteSize()
	if sz == 0 {
		return 0, fmt.Errorf("unsupported scalar type: %s", k)
	}

	var u64 uint64
	switch k {
	case Int8:
		u64 = uint64(*(*int8)(ptr))
	case Uint8, Byte:
		u64 = uint64(*(*uint8)(ptr))
	case Int16:
		u64 = uint64(*(*int16)(ptr))
	case Uint16, Word:
		u64 = uint64(*(*uint16)(ptr))
	case Int32:
		u64 = uint64(*(*int32)(ptr))
	case Uint32, Dword:
		u64 = uint64(*(*uint32)(ptr))
	case Int64:
		u64 = uint64(*(*int64)(ptr))
	case Uint64, Qword:
		u64 = *(*uint64)(ptr)
	case Float32:
		u64 = uint64(math.Float32bits(*(*float32)(ptr)))
	case Float64:
		u64 = math.Float64bits(*(*float64)(ptr))
	default:
		return 0, fmt.Errorf("unsupported scalar type: %s", k)
	}

	return writeU64(w, order, u64, sz)
}

func (ms *Marshaler) unsafeReadStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
	typ := strc.Type()
	meta, err := getStructMetadata(typ)
	if err != nil {
		return 0, err
	}
	// A struct-level byte order overrides the inherited order for this struct's
	// fields; per-field endian= still overrides it in turn.
	order = resolveByteOrder(order, meta.endian)

	var base unsafe.Pointer
	if strc.CanAddr() {
		base = unsafe.Pointer(strc.Addr().Pointer())
	} else {
		copyVal := reflect.New(strc.Type()).Elem()
		copyVal.Set(strc)
		base = unsafe.Pointer(copyVal.Addr().Pointer())
	}
	firstElem := true
	wErr := func(i int, e error) error {
		if firstElem && (errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF)) {
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
		if fMeta.ignore || fMeta.unexported {
			continue
		}

		if fMeta.fieldErr != nil && !fMeta.hasTag {
			err = wErr(fMeta.index, fMeta.fieldErr)
			return
		}

		// check omittable expr
		if fMeta.omittable && fMeta.omittableExpr != "" {
			limit, errEval := evaluateTagValue(strc, fMeta.omittableExpr)
			if errEval == nil && n >= limit {
				break
			}
		}

		fieldPtr := unsafe.Add(base, fMeta.offset)
		currType := typ.Field(fMeta.index).Type
		// dereference/allocate pointers
		currPtr := fieldPtr
		wasNilPtr := false
		indirectCount := 0
		for currType.Kind() == reflect.Ptr || currType.Kind() == reflect.Interface {
			indirectCount++
			if currType.Kind() == reflect.Ptr {
				valPtr := *(*unsafe.Pointer)(currPtr)
				if valPtr == nil {
					wasNilPtr = true
					elemType := currType.Elem()
					newVal := reflect.New(elemType)
					*(*unsafe.Pointer)(currPtr) = unsafe.Pointer(newVal.Pointer())
				}
				currPtr = *(*unsafe.Pointer)(currPtr)
				currType = currType.Elem()
			} else {
				break
			}
		}

		// If it's interface or has custom codec, fall back to reflection
		if typ.Field(fMeta.index).Type.Kind() == reflect.Interface || fMeta.codec != "" {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType, option := getNaturalType(fieldVal)
			if fMeta.hasTag {
				if fMeta.encodeType != Any {
					naturalType = fMeta.encodeType
				}
				if fMeta.isArray {
					option.isArray = true
					if fMeta.arrayLenExpr != "" {
						option.arrayLen, err = evaluateTagValue(strc, fMeta.arrayLenExpr)
						if err != nil {
							return n, wErr(fMeta.index, err)
						}
					}
				}
				if fMeta.bufLenExpr != "" {
					option.bufLen, err = evaluateTagValue(strc, fMeta.bufLenExpr)
					if err != nil {
						return n, wErr(fMeta.index, err)
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
			m, err = ms.readMain(r, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
					if wasNilPtr {
						strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
					}
					err = nil
					break
				}
				return n, wErr(fMeta.index, err)
			}
			if err = validateField(fieldVal, &fMeta); err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			firstElem = false
			continue
		}

		fieldOrder := resolveByteOrder(order, fMeta.endian)

		// Check if it's a nested struct
		fieldValType := typ.Field(fMeta.index).Type
		for i := 0; i < indirectCount; i++ {
			fieldValType = fieldValType.Elem()
		}

		if fieldValType.Kind() == reflect.Struct {
			structVal := reflect.NewAt(fieldValType, currPtr).Elem()
			var m int
			m, err = ms.unsafeReadStruct(r, fieldOrder, structVal)
			if err != nil {
				if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
					if wasNilPtr {
						strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
					}
					err = nil
					break
				}
				return n, wErr(fMeta.index, err)
			}
			if err = validateField(structVal, &fMeta); err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			firstElem = false
			continue
		}

		// Handle array/slice fields
		if fMeta.isArray || fieldValType.Kind() == reflect.Slice || fieldValType.Kind() == reflect.Array {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType := fMeta.naturalType
			option := fMeta.option
			if fMeta.arrayLenExpr != "" {
				option.arrayLen, err = evaluateTagValue(strc, fMeta.arrayLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			} else if fieldValType.Kind() == reflect.Slice {
				sh := (*sliceHeader)(currPtr)
				option.arrayLen = sh.Len
			}
			if fMeta.bufLenExpr != "" {
				option.bufLen, err = evaluateTagValue(strc, fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			var ok bool
			if fieldValType.Kind() == reflect.Slice || fieldValType.Kind() == reflect.Array {
				sliceVal := reflect.NewAt(fieldValType, currPtr).Elem()
				m, ok, err = ms.unsafeReadSlice(r, fieldOrder, currPtr, sliceVal, fieldValType.Kind() == reflect.Slice, option.arrayLen, fMeta.naturalType, fieldValType.Elem())
				if err != nil {
					if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
						if wasNilPtr {
							strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
						}
						err = nil
						break
					}
					return n, wErr(fMeta.index, err)
				}
			}
			if ok {
				if err = validateField(fieldVal, &fMeta); err != nil {
					return n, wErr(fMeta.index, err)
				}
				n += m
				firstElem = false
				continue
			}
			m, err = ms.readMain(r, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
					if wasNilPtr {
						strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
					}
					err = nil
					break
				}
				return n, wErr(fMeta.index, err)
			}
			if err = validateField(fieldVal, &fMeta); err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			firstElem = false
			continue
		}

		// Handle string fields
		if fieldValType.Kind() == reflect.String {
			var m int
			fieldVal := strc.Field(fMeta.index)
			naturalType := fMeta.naturalType
			option := fMeta.option
			if fMeta.bufLenExpr != "" {
				option.bufLen, err = evaluateTagValue(strc, fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			m, err = ms.readMain(r, order, fieldVal, naturalType, option, strc, fMeta.index)
			if err != nil {
				if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
					if wasNilPtr {
						strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
					}
					err = nil
					break
				}
				return n, wErr(fMeta.index, err)
			}
			if err = validateField(fieldVal, &fMeta); err != nil {
				return n, wErr(fMeta.index, err)
			}
			n += m
			firstElem = false
			continue
		}

		// Handle padding
		if fMeta.encodeType == Pad {
			l := 1
			if fMeta.bufLenExpr != "" {
				l, err = evaluateTagValue(strc, fMeta.bufLenExpr)
				if err != nil {
					return n, wErr(fMeta.index, err)
				}
			}
			var m int
			m, err = skipBytes(r, l)
			if err != nil {
				if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
					err = nil
					break
				}
				return n, wErr(fMeta.index, err)
			}
			n += m
			firstElem = false
			continue
		}

		// Handle basic scalar fields using unsafe
		var m int
		m, err = ms.unsafeReadScalar(r, fieldOrder, currPtr, fMeta.encodeType, fieldValType.Kind())
		if err != nil {
			if fMeta.omittable && (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 {
				if wasNilPtr {
					strc.Field(fMeta.index).Set(reflect.Zero(strc.Field(fMeta.index).Type()))
				}
				err = nil
				break
			}
			return n, wErr(fMeta.index, err)
		}
		if err = validateField(strc.Field(fMeta.index), &fMeta); err != nil {
			return n, wErr(fMeta.index, err)
		}
		n += m
		firstElem = false
	}
	return
}

func (ms *Marshaler) unsafeReadScalar(r io.Reader, order ByteOrder, ptr unsafe.Pointer, encodeType eType, goKind reflect.Kind) (n int, err error) {
	k := encodeType
	if k == Any || k == iInvalid {
		k = getITypeFromRKind(goKind)
	}

	sz := k.ByteSize()
	if sz == 0 {
		return 0, fmt.Errorf("unsupported scalar type: %s", k)
	}

	var u64 uint64
	u64, n, err = readU64(r, order, sz)
	if err != nil {
		return
	}

	switch k {
	case Int8:
		*(*int8)(ptr) = int8(u64)
	case Uint8, Byte:
		*(*uint8)(ptr) = uint8(u64)
	case Int16:
		*(*int16)(ptr) = int16(u64)
	case Uint16, Word:
		*(*uint16)(ptr) = uint16(u64)
	case Int32:
		*(*int32)(ptr) = int32(u64)
	case Uint32, Dword:
		*(*uint32)(ptr) = uint32(u64)
	case Int64:
		*(*int64)(ptr) = int64(u64)
	case Uint64, Qword:
		*(*uint64)(ptr) = u64
	case Float32:
		*(*float32)(ptr) = math.Float32frombits(uint32(u64))
	case Float64:
		*(*float64)(ptr) = math.Float64frombits(u64)
	default:
		return 0, fmt.Errorf("unsupported scalar type: %s", k)
	}

	return
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

func swapBytes(buf []byte, sz int) {
	if sz == 2 {
		simdSwap16(buf)
	} else if sz == 4 {
		simdSwap32(buf)
	} else if sz == 8 {
		simdSwap64(buf)
	}
}

func (ms *Marshaler) unsafeWriteSlice(w io.Writer, fieldOrder ByteOrder, currPtr unsafe.Pointer, isSlice bool, arrayLen int, elType eType, goElType reflect.Type) (n int, ok bool, err error) {
	if !isCompatibleFastPath(goElType, elType) {
		return 0, false, nil
	}

	sz := elType.ByteSize()
	if sz == 0 {
		return 0, false, nil
	}

	var dataPtr unsafe.Pointer
	var length int
	if isSlice {
		sh := (*sliceHeader)(currPtr)
		if sh.Len == 0 {
			if arrayLen > 0 {
				var m int
				m, err = zeroFill(w, arrayLen*sz)
				return m, true, err
			}
			return 0, true, nil
		}
		dataPtr = sh.Data
		length = sh.Len
	} else {
		dataPtr = currPtr
		length = arrayLen
	}

	if length == 0 {
		return 0, true, nil
	}

	totalBytes := length * sz
	byteSlice := unsafe.Slice((*byte)(dataPtr), totalBytes)

	var written int
	if sz == 1 || fieldOrder == hostEndian {
		var m int
		m, err = w.Write(byteSlice)
		if err != nil {
			return m, true, err
		}
		written = m
	} else {
		buf := make([]byte, totalBytes)
		copy(buf, byteSlice)
		swapBytes(buf, sz)
		var m int
		m, err = w.Write(buf)
		if err != nil {
			return m, true, err
		}
		written = m
	}

	if isSlice && length < arrayLen {
		padBytes := (arrayLen - length) * sz
		var m int
		m, err = zeroFill(w, padBytes)
		if err != nil {
			return written + m, true, err
		}
		written += m
	}

	return written, true, nil
}

func (ms *Marshaler) unsafeReadSlice(r io.Reader, fieldOrder ByteOrder, currPtr unsafe.Pointer, fieldVal reflect.Value, isSlice bool, arrayLen int, elType eType, goElType reflect.Type) (n int, ok bool, err error) {
	if !isCompatibleFastPath(goElType, elType) {
		return 0, false, nil
	}

	sz := elType.ByteSize()
	if sz == 0 {
		return 0, false, nil
	}

	var dataPtr unsafe.Pointer
	var length int

	if isSlice {
		sh := (*sliceHeader)(currPtr)
		if sh.Data == nil {
			if arrayLen == 0 {
				return 0, true, nil
			}
			newS := reflect.MakeSlice(fieldVal.Type(), arrayLen, arrayLen)
			fieldVal.Set(newS)
			sh = (*sliceHeader)(currPtr)
		} else if arrayLen == 0 {
			arrayLen = sh.Len
		} else if sh.Len < arrayLen {
			newS := reflect.MakeSlice(fieldVal.Type(), arrayLen, arrayLen)
			reflect.Copy(newS, fieldVal)
			fieldVal.Set(newS)
			sh = (*sliceHeader)(currPtr)
		}
		length = arrayLen
		if length == 0 {
			return 0, true, nil
		}
		dataPtr = sh.Data
	} else {
		length = arrayLen
		dataPtr = currPtr
	}

	if length == 0 {
		return 0, true, nil
	}

	totalBytes := length * sz
	byteSlice := unsafe.Slice((*byte)(dataPtr), totalBytes)

	n, err = io.ReadFull(r, byteSlice)
	if err != nil {
		return n, true, err
	}

	if sz > 1 && fieldOrder != hostEndian {
		swapBytes(byteSlice, sz)
	}

	return n, true, nil
}

func isCompatibleFastPath(goElType reflect.Type, elType eType) bool {
	goKind := goElType.Kind()
	elKind := elType.iKind()
	sz := elType.ByteSize()
	goSz := int(goElType.Size())

	if sz != goSz {
		return false
	}

	switch goKind {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return elKind == intKind || elKind == bitmapKind
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return elKind == uintKind || elKind == bitmapKind
	case reflect.Float32:
		return elType == Float32
	case reflect.Float64:
		return elType == Float64
	}
	return false
}
