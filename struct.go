// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const (
	tagName = "binary"
)

var (
	errNegativeSize = errors.New("the size must not be negative")

	// regexp to match a tag
	mTag = regexp.MustCompile(`^\s*(\[([^\]]*)\])?([^\s\(\)]*)(\(([^\)]+)\))?`)

	// single entry of tag-value evaluation
	mExpression = regexp.MustCompile(`\s*([\+\-])?\s*([^\s\+\-]+)`)
)

// simple add-sub calculator, with struct field referencing
func evaluateTagValue(strc reflect.Value, stmt string) (value int, err error) {

	type entry struct {
		operation string
		value     string
	}
	poly := make([]entry, 0)

	m := mExpression.FindAllStringSubmatchIndex(stmt, -1)
	for _, n := range m {
		e := entry{}
		if n[2] >= 0 {
			e.operation = stmt[n[2]:n[3]]
		}
		e.value = stmt[n[4]:n[5]]
		poly = append(poly, e)
	}

	printerr := func(s string) error {
		return fmt.Errorf("invalid argument %s", s)
	}

	var sum int64
	for _, q := range poly {
		// try to evaluate single expression as a interger value
		var i64 int64
		i64, e := strconv.ParseInt(q.value, 0, 64)
		if e != nil {
			// try to reference a struct member variable
			if strc.Kind() != reflect.Struct {
				// given data is not a struct
				err = printerr(q.value)
				return
			}
			typ := strc.Type()
			f, ok := typ.FieldByName(q.value)
			if !ok {
				// no such field name
				err = printerr(q.value)
				return
			}
			v := strc.FieldByIndex(f.Index)
			if !v.Type().ConvertibleTo(i64type) {
				// the field cannot be converted to an integer
				err = printerr(q.value)
				return
			}
			i64 = v.Convert(i64type).Int()
		}

		switch q.operation {
		case "+", "":
			sum = sum + i64
		case "-":
			sum = sum - i64
		default:
			err = fmt.Errorf("invalid operation <%s>", q.operation)
			return
		}
	}
	value = int(sum)
	return
}

// read struct tag
func parseStructField(structType reflect.Type, strc reflect.Value, i int) (encodeType eType, option typeOption, err error) {

	field := structType.Field(i)
	fType := field.Type
	fKind := fType.Kind()

	// check field type
	var fieldErr error
	switch fKind {
	case reflect.Invalid:
		fieldErr = fmt.Errorf("invalid data type")
	case reflect.Complex64, reflect.Complex128:
		fieldErr = fmt.Errorf("complex type not supported")
	case reflect.UnsafePointer:
		fieldErr = fmt.Errorf("pointer type not supported")
	case reflect.Chan, reflect.Func, reflect.Map:
		fieldErr = fmt.Errorf("unsupported type: %v", fType.Kind())
	default:
		encodeType, option = getNaturalType(strc.Field(i))
	}

	// read the tag
	tags := strings.Split(field.Tag.Get(tagName), ",")
	if len(tags) == 0 || tags[0] == "" {
		// no tags to process
		if fieldErr != nil {
			err = fieldErr
		}
		return
	}

	m := mTag.FindStringSubmatch(tags[0])
	typeTag := m[3]
	parsedType := Any
	if typeTag != "" {
		parsedType = typeByName(typeTag)
	}
	if encodeType == iInvalid && (parsedType != Pad && parsedType != Ignore) {
		// value type is unknown and target type is not an ignoring type
		if fieldErr != nil {
			// field type is non-encodable
			err = fieldErr
		} else {
			err = fmt.Errorf("the field %s is not encodable", field.Name)
		}
		return
	}
	encodeType = parsedType

	// check for array type and its size
	option.isArray = m[1] != ""
	if option.isArray && m[2] != "" {
		option.arrayLen, err = evaluateTagValue(strc, m[2])
		if err != nil {
			return
		}
		if option.arrayLen < 0 {
			err = errNegativeSize
			return
		}
	}

	if m[5] != "" {
		option.bufLen, err = evaluateTagValue(strc, m[5])
		if option.bufLen == 0 {
			err = fmt.Errorf("element size must not be zero")
			return
		}
		if option.bufLen < 0 {
			err = errNegativeSize
			return
		}
	}

	for i := 1; i < len(tags); i++ {
		t := strings.Split(tags[i], "=")
		for j := 0; j < len(t); j++ {
			t[i] = strings.TrimSpace(t[i])
		}
		switch t[0] {
		case "encoding":
			option.encoding = t[1]

		default:
			err = fmt.Errorf("unknown tag %s", t[0])
			return
		}
	}

	// binary: "ignore"		// ignore

	// binary: "type"
	// binary: "[size]type"
	// binary: "[size]any"

	// binary: "zstring[,encoding=ENC]"	// zero-terminated string
	// binary: "zstring(size)[,encoding=ENC]"	// zero-terminated string with fixed size
	// binary: "bstring[,encoding=ENC]"	// byte len + []byte
	// binary: "wstring[,encoding=ENC]"	// word len + []byte
	// binary: "dwstring[,encoding=ENC]"	// dword len + []byte

	return
}
