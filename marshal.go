package binarystruct

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
//ErrInvalidType = fmt.Errorf("invalid type")
)

func Write(w io.Writer, order ByteOrder, data interface{}) (err error) {
	return write(w, order, reflect.ValueOf(data))
}

func write(w io.Writer, order ByteOrder, v reflect.Value) (err error) {

	t := reflect.TypeOf(v)

	fmt.Println(t.Kind())

	switch t.Kind() {

	case reflect.Struct:
		//case reflect.Array:
		return writeStruct(w, order, v)

	default:
		return binary.Write(w, order, v)
	}

	return ErrInvalidType
}

func writeSingle(w io.Writer, order ByteOrder, v reflect.Value, k Type) (n int, err error) {
	sz := k.ByteSize()
	if sz <= 0 {
		// String kind or invalid kind
		err = ErrInvalidType
		return
	}

	f := encodeFunc(v.Type(), k)
	u64, _, err := f(v)

	var buf [8]byte
	b := buf[:sz]

	switch sz {
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

	n, err = w.Write(b)
	return
}

func writeStruct(w io.Writer, order ByteOrder, v reflect.Value) (err error) {
	/*
		t := v.Type()
		nField := t.NumField()
		for i:=0; i<nField; i++ {
			binType, isArray, arraySize, e := parseStructField(t, v, i)
			if e != nil {
				err = e
				return
			}
			if isArray {
				field := t.Field(i)
				switch field.Type {
				case reflect.Array:
				case reflect.Slice:
				default:
					err = fmt.Errorf("underlying field must be slice or array")
					return
				}
				sz := v.Len()
				if sz>=arraySize {
					sz = arraySize
				}
				for i:=0; i<sz; i++ {
					err = writeSingle(w, order, v,
				}
			} else {

			}
		}
	*/

	return
}

var (
	mTag = regexp.MustCompile(`^\s*(\[([^\]]+)\])?([^\s]*)`)
)

//func parseType(field reflect.StructField) {
func parseStructField(t reflect.Type, v reflect.Value, i int) (kind Type, isArray bool, arraySize int, err error) {

	field := t.Field(i)

	fType := field.Type

	// check field type
	switch fType.Kind() {
	case reflect.Invalid:
		err = fmt.Errorf("invalid type")
	case reflect.Complex64, reflect.Complex128:
		err = fmt.Errorf("complex type not supported")
	case reflect.Ptr, reflect.UnsafePointer:
		err = fmt.Errorf("pointer type not supported")
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map:
		err = fmt.Errorf("unsupported type: %v", fType.Kind())
	}
	if err != nil {
		return
	}

	tags := strings.Split(field.Tag.Get("binary"), ",")
	if len(tags) == 0 {
		if fType.Kind() == reflect.Array {
			isArray = true
			//binType = t.Elem()
			arraySize = t.Len()
		}
		return
	}

	m := mTag.FindStringSubmatch(tags[0])
	typeTag := m[3]
	if typeTag != "" {
		kind = GetType(typeTag)
	}
	isArray = m[1] != ""
	arraySizeTag := m[2]

	// TODO: parse arraySizeTag and do some math
	arraySize, _ = strconv.Atoi(arraySizeTag)

	// binary: ""		// ignore
	// binary: "-"		// ignore

	// binary: "type"
	// binary: "[size]type"

	// binary: "bstring[,encoding]"	// byte len + []byte
	// binary: "wstring[,encoding]"	// word len + []byte
	// binary: "dwstring[,encoding]"	// dword len + []byte
	// binary: "zstring[,encoding]"		// zero-terminated string
	// binary: "[size]zstring[,encoding]"	// zero-terminated string of max size size

	return
}
