package binarystruct

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func parseStructField(structType reflect.Type, strc reflect.Value, i int) (encodeType Type, option typeOption, err error) {

	field := structType.Field(i)
	fType := field.Type

	// check field type
	switch fType.Kind() {
	case reflect.Invalid:
		err = fmt.Errorf("invalid data type")
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

	fValue := strc.Field(i)
	encodeType, option = getNaturalType(fValue)
	if encodeType == Invalid {
		err = fmt.Errorf("invalid encoding type")
		return
	}

	tags := strings.Split(field.Tag.Get("binary"), ",")
	if len(tags) == 0 || tags[0] == "" {
		// no tags to process
		return
	}

	m := mTag.FindStringSubmatch(tags[0])
	typeTag := m[3]
	if typeTag != "" {
		encodeType = TypeByName(typeTag)
	}
	option.isArray = m[1] != ""
	arraySizeTag := m[2]

	// TODO: parse arraySizeTag and do some math
	option.arrayLen, _ = strconv.Atoi(arraySizeTag)

	// binary: ""		// ignore
	// binary: "-"		// ignore

	// binary: "type"
	// binary: "[size]type"
	// binary: "[size]any"

	// binary: "bstring[,encoding=ENC]"	// byte len + []byte
	// binary: "wstring[,encoding=ENC]"	// word len + []byte
	// binary: "dwstring[,encoding=ENC]"	// dword len + []byte
	// binary: "zstring[,encoding=ENC]"		// zero-terminated string
	// binary: "[size]zstring[,encoding=ENC]"	// zero-terminated string of max size size

	return
}
