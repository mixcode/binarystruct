// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	tagName = "binary"
)

type structFieldMetadata struct {
	index         int
	name          string
	offset        uintptr
	hasTag        bool
	encodeType    eType
	isArray       bool
	arrayLenExpr  string
	bufLenExpr    string
	valueofExpr   string
	encoding      string
	endian        endianOverride
	serializer    string
	ignore        bool
	unexported    bool
	fieldErr      error
	omittable     bool
	omittableExpr string
	naturalType   eType
	option        typeOption
	hasRange      bool
	rangeMin      float64
	rangeMax      float64
	hasRangeMin   bool
	hasRangeMax   bool
	hasMatch      bool
	matchPattern  string
	matchRegexp   *regexp.Regexp
}

type structMetadata struct {
	fields []structFieldMetadata
}

// fieldByName returns the metadata for the field with the given Go name.
func (m *structMetadata) fieldByName(name string) (structFieldMetadata, bool) {
	for _, f := range m.fields {
		if f.name == name {
			return f, true
		}
	}
	return structFieldMetadata{}, false
}

var (
	errNegativeSize = errors.New("the size must not be negative")

	// regexp to match a tag
	mTag = regexp.MustCompile(`^\s*(\[([^\]]*)\])?([^\s\(\)]*)(\(([^\)]+)\))?`)

	// single entry of tag-value evaluation
	mExpression = regexp.MustCompile(`\s*([\+\-])?\s*([^\s\+\-]+)`)

	structMetadataCache sync.Map // map[reflect.Type]*structMetadata
)

type tokenType int

const (
	tokEOF tokenType = iota
	tokNum
	tokIdent
	tokPlus
	tokMinus
	tokMul
	tokDiv
	tokLParen
	tokRParen
)

type token struct {
	typ tokenType
	val string
}

func tokenize(expr string) ([]token, error) {
	var tokens []token
	i := 0
	n := len(expr)
	for i < n {
		c := expr[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		if c == '+' {
			tokens = append(tokens, token{tokPlus, "+"})
			i++
			continue
		}
		if c == '-' {
			tokens = append(tokens, token{tokMinus, "-"})
			i++
			continue
		}
		if c == '*' {
			tokens = append(tokens, token{tokMul, "*"})
			i++
			continue
		}
		if c == '/' {
			tokens = append(tokens, token{tokDiv, "/"})
			i++
			continue
		}
		if c == '(' {
			tokens = append(tokens, token{tokLParen, "("})
			i++
			continue
		}
		if c == ')' {
			tokens = append(tokens, token{tokRParen, ")"})
			i++
			continue
		}
		if c >= '0' && c <= '9' {
			start := i
			if i+1 < n && expr[i] == '0' && (expr[i+1] == 'x' || expr[i+1] == 'X' || expr[i+1] == 'o' || expr[i+1] == 'O' || expr[i+1] == 'b' || expr[i+1] == 'B') {
				i += 2
			}
			for i < n && ((expr[i] >= '0' && expr[i] <= '9') || (expr[i] >= 'a' && expr[i] <= 'f') || (expr[i] >= 'A' && expr[i] <= 'F') || expr[i] == '_') {
				i++
			}
			tokens = append(tokens, token{tokNum, expr[start:i]})
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			start := i
			i++
			for i < n && ((expr[i] >= 'a' && expr[i] <= 'z') || (expr[i] >= 'A' && expr[i] <= 'Z') || (expr[i] >= '0' && expr[i] <= '9') || expr[i] == '_') {
				i++
			}
			tokens = append(tokens, token{tokIdent, expr[start:i]})
			continue
		}
		return nil, fmt.Errorf("unexpected character: %c", c)
	}
	tokens = append(tokens, token{tokEOF, ""})
	return tokens, nil
}

type tagParser struct {
	tokens []token
	pos    int
	strc   reflect.Value

	// resolveIdent resolves a bare field reference (e.g. "PayloadSize").
	// When nil, bare field references are rejected.
	resolveIdent func(name string) (int, error)
	// callFunc resolves a function call such as bytelen(Name) or count(Items).
	// When nil, function calls are rejected (the case for decode-side size
	// expressions, where functions are not permitted).
	callFunc func(funcName, argName string) (int, error)
}

func (p *tagParser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{tokEOF, ""}
	}
	return p.tokens[p.pos]
}

func (p *tagParser) consume() token {
	t := p.peek()
	if t.typ != tokEOF {
		p.pos++
	}
	return t
}

func (p *tagParser) parseExpr() (int, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		t := p.peek()
		if t.typ == tokPlus {
			p.consume()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val = val + r
		} else if t.typ == tokMinus {
			p.consume()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val = val - r
		} else {
			break
		}
	}
	return val, nil
}

func (p *tagParser) parseTerm() (int, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		t := p.peek()
		if t.typ == tokMul {
			p.consume()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			val = val * r
		} else if t.typ == tokDiv {
			p.consume()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val = val / r
		} else {
			break
		}
	}
	return val, nil
}

func (p *tagParser) parseFactor() (int, error) {
	t := p.peek()
	if t.typ == tokPlus {
		p.consume()
		return p.parseFactor()
	}
	if t.typ == tokMinus {
		p.consume()
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	if t.typ == tokLParen {
		p.consume()
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.consume().typ != tokRParen {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		return val, nil
	}
	if t.typ == tokNum {
		p.consume()
		i64, err := strconv.ParseInt(t.val, 0, 64)
		if err != nil {
			return 0, err
		}
		return int(i64), nil
	}
	if t.typ == tokIdent {
		p.consume()
		// function call: IDENT '(' IDENT ')'
		if p.peek().typ == tokLParen {
			p.consume() // '('
			arg := p.consume()
			if arg.typ != tokIdent {
				return 0, fmt.Errorf("function %s() expects a field name argument", t.val)
			}
			if p.consume().typ != tokRParen {
				return 0, fmt.Errorf("missing closing parenthesis in %s(%s", t.val, arg.val)
			}
			if p.callFunc == nil {
				return 0, fmt.Errorf("function %s() is not allowed here (functions are valid only in valueof)", t.val)
			}
			return p.callFunc(t.val, arg.val)
		}
		if p.resolveIdent == nil {
			return 0, fmt.Errorf("field reference %s is not allowed here", t.val)
		}
		return p.resolveIdent(t.val)
	}
	return 0, fmt.Errorf("unexpected token %s", t.val)
}

// evaluateTagValue evaluates arithmetic expressions for struct field tagging.
func evaluateTagValue(strc reflect.Value, stmt string) (value int, err error) {
	tokens, err := tokenize(stmt)
	if err != nil {
		return 0, err
	}
	p := &tagParser{
		tokens:       tokens,
		strc:         strc,
		resolveIdent: fieldValueResolver(strc),
		// callFunc stays nil: bytelen()/count() are not permitted in
		// arithmetic decode-side expressions ([arrayLen] and buf_len).
	}
	value, err = p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.peek().typ != tokEOF {
		return 0, fmt.Errorf("unexpected token at end of expression: %s", p.peek().val)
	}
	return value, nil
}

// fieldValueResolver returns a resolver that reads a sibling field's integer
// value from strc, for use by arithmetic and decode-side size expressions.
func fieldValueResolver(strc reflect.Value) func(string) (int, error) {
	return func(name string) (int, error) {
		if strc.Kind() != reflect.Struct {
			return 0, fmt.Errorf("cannot reference field %s of non-struct", name)
		}
		typ := strc.Type()
		f, ok := typ.FieldByName(name)
		if !ok {
			return 0, fmt.Errorf("no field named %s", name)
		}
		v := strc.FieldByIndex(f.Index)
		if !v.Type().ConvertibleTo(i64type) {
			return 0, fmt.Errorf("field %s is not convertible to integer", name)
		}
		return int(v.Convert(i64type).Int()), nil
	}
}

type exprFuncCall struct {
	name string // "bytelen" or "count"
	arg  string // referenced field name
}

// exprReferences parses expr WITHOUT evaluating it, returning the field names
// it references and the function calls it makes. Used at metadata-build time to
// validate valueof expressions and to reject functions in decode expressions.
func exprReferences(expr string) (refs []string, funcs []exprFuncCall, err error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return nil, nil, err
	}
	p := &tagParser{
		tokens: tokens,
		resolveIdent: func(name string) (int, error) {
			refs = append(refs, name)
			return 0, nil
		},
		callFunc: func(fn, arg string) (int, error) {
			funcs = append(funcs, exprFuncCall{name: fn, arg: arg})
			refs = append(refs, arg)
			return 0, nil
		},
	}
	if _, err = p.parseExpr(); err != nil {
		return nil, nil, err
	}
	if p.peek().typ != tokEOF {
		return nil, nil, fmt.Errorf("unexpected token at end of expression: %s", p.peek().val)
	}
	return refs, funcs, nil
}

// parse tag string directly
func parseTagString(tagStr string, strc reflect.Value, naturalType eType, naturalOption typeOption, fieldErr error) (encodeType eType, option typeOption, err error) {
	encodeType = naturalType
	option = naturalOption

	// read the tag
	tags := strings.Split(tagStr, ",")
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
			err = fmt.Errorf("the value is not encodable")
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
		if option.bufLen < 0 {
			err = errNegativeSize
			return
		}
	}

	for i := 1; i < len(tags); i++ {
		t := strings.Split(tags[i], "=")
		for j := 0; j < len(t); j++ {
			t[j] = strings.TrimSpace(t[j])
		}
		switch t[0] {
		case "encoding":
			if len(t) > 1 {
				option.encoding = t[1]
			} else {
				err = fmt.Errorf("missing value for encoding tag")
				return
			}
		case "endian":
			if len(t) > 1 {
				switch strings.ToLower(t[1]) {
				case "big":
					option.endian = endianBig
				case "little":
					option.endian = endianLittle
				case "inverse":
					option.endian = endianInverse
				default:
					err = fmt.Errorf("unknown endian value: %s", t[1])
					return
				}
			} else {
				err = fmt.Errorf("missing value for endian tag")
				return
			}
		case "serializer":
			if len(t) > 1 {
				option.serializer = t[1]
			} else {
				err = fmt.Errorf("missing value for serializer tag")
				return
			}
		case "valueof":
			err = fmt.Errorf("valueof is only supported on struct fields, not single values")
			return

		default:
			err = fmt.Errorf("unknown tag %s", t[0])
			return
		}
	}

	return
}

// getStructMetadata builds or retrieves cached metadata for the struct type.
// getStructMetadata builds or retrieves cached metadata for the struct type.
func getStructMetadata(structType reflect.Type) (*structMetadata, error) {
	if val, ok := structMetadataCache.Load(structType); ok {
		return val.(*structMetadata), nil
	}

	nField := structType.NumField()
	fields := make([]structFieldMetadata, 0, nField)

	// fieldName -> field names referenced by its valueof expression,
	// collected for reference-cycle detection after all fields are parsed.
	valueofRefs := make(map[string][]string)

	for i := 0; i < nField; i++ {
		field := structType.Field(i)
		fType := field.Type
		fKind := fType.Kind()

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
		}

		tagStr := field.Tag.Get(tagName)
		tags := strings.Split(tagStr, ",")

		meta := structFieldMetadata{
			index:    i,
			name:     field.Name,
			offset:   field.Offset,
			fieldErr: fieldErr,
		}
		meta.naturalType, meta.option = getStaticTypeInfo(field.Type)

		name := field.Name
		if len(name) == 0 || strings.ToUpper(name)[0] != name[0] {
			meta.unexported = true
		}

		if len(tags) == 0 || tags[0] == "" {
			fields = append(fields, meta)
			continue
		}

		meta.hasTag = true
		m := mTag.FindStringSubmatch(tags[0])
		typeTag := m[3]
		parsedType := Any
		if typeTag != "" {
			parsedType = typeByName(typeTag)
		}
		meta.encodeType = parsedType

		if parsedType == Ignore {
			meta.ignore = true
			fields = append(fields, meta)
			continue
		}

		meta.isArray = m[1] != ""
		if meta.isArray && m[2] != "" {
			meta.arrayLenExpr = m[2]
		}

		if m[5] != "" {
			meta.bufLenExpr = m[5]
		}

		// parse options
		for idx := 1; idx < len(tags); idx++ {
			t := strings.Split(tags[idx], "=")
			for j := 0; j < len(t); j++ {
				t[j] = strings.TrimSpace(t[j])
			}
			switch t[0] {
			case "encoding":
				if len(t) > 1 {
					meta.encoding = t[1]
				} else {
					return nil, fmt.Errorf("missing value for encoding tag on field %s", field.Name)
				}
			case "endian":
				if len(t) > 1 {
					switch strings.ToLower(t[1]) {
					case "big":
						meta.endian = endianBig
					case "little":
						meta.endian = endianLittle
					case "inverse":
						meta.endian = endianInverse
					default:
						return nil, fmt.Errorf("unknown endian value: %s on field %s", t[1], field.Name)
					}
				} else {
					return nil, fmt.Errorf("missing value for endian tag on field %s", field.Name)
				}
			case "serializer":
				if len(t) > 1 {
					meta.serializer = t[1]
				} else {
					return nil, fmt.Errorf("missing value for serializer tag on field %s", field.Name)
				}
			case "omittable":
				meta.omittable = true
				if len(t) > 1 {
					meta.omittableExpr = t[1]
				}
			case "valueof":
				if len(t) > 1 {
					meta.valueofExpr = strings.Join(t[1:], "=")
				} else {
					return nil, fmt.Errorf("missing value for valueof tag on field %s", field.Name)
				}
			case "range":
				if len(t) > 1 {
					meta.hasRange = true
					bounds := strings.Split(t[1], "..")
					if len(bounds) != 2 {
						return nil, fmt.Errorf("invalid range format on field %s; must be min..max", field.Name)
					}
					minStr := strings.TrimSpace(bounds[0])
					maxStr := strings.TrimSpace(bounds[1])
					if minStr != "" {
						minVal, errParse := strconv.ParseFloat(minStr, 64)
						if errParse != nil {
							return nil, fmt.Errorf("invalid range min value on field %s: %w", field.Name, errParse)
						}
						meta.rangeMin = minVal
						meta.hasRangeMin = true
					}
					if maxStr != "" {
						maxVal, errParse := strconv.ParseFloat(maxStr, 64)
						if errParse != nil {
							return nil, fmt.Errorf("invalid range max value on field %s: %w", field.Name, errParse)
						}
						meta.rangeMax = maxVal
						meta.hasRangeMax = true
					}
				} else {
					return nil, fmt.Errorf("missing value for range tag on field %s", field.Name)
				}
			case "match":
				if len(t) > 1 {
					meta.hasMatch = true
					meta.matchPattern = t[1]
					re, errCompile := regexp.Compile(meta.matchPattern)
					if errCompile != nil {
						return nil, fmt.Errorf("invalid regex pattern %q on field %s: %w", meta.matchPattern, field.Name, errCompile)
					}
					meta.matchRegexp = re
				} else {
					return nil, fmt.Errorf("missing value for match tag on field %s", field.Name)
				}
			default:
				return nil, fmt.Errorf("unknown tag %s on field %s", t[0], field.Name)
			}
		}

		if meta.hasTag {
			if meta.encodeType != Any {
				meta.naturalType = meta.encodeType
			}
			if meta.isArray {
				meta.option.isArray = true
				if meta.arrayLenExpr != "" {
					if val, err := evaluateTagValue(reflect.Value{}, meta.arrayLenExpr); err == nil {
						meta.option.arrayLen = val
					}
				}
			}
			if meta.bufLenExpr != "" {
				if val, err := evaluateTagValue(reflect.Value{}, meta.bufLenExpr); err == nil {
					meta.option.bufLen = val
				}
			}
			if meta.encoding != "" {
				meta.option.encoding = meta.encoding
			}
			if meta.endian != endianNone {
				meta.option.endian = meta.endian
			}
			if meta.serializer != "" {
				meta.option.serializer = meta.serializer
			}

			// Decode-side size expressions must be arithmetic only: reject
			// bytelen()/count() in [arrayLen] and buf_len.
			for _, e := range []string{meta.arrayLenExpr, meta.bufLenExpr} {
				if e == "" {
					continue
				}
				if _, fns, errRef := exprReferences(e); errRef == nil && len(fns) > 0 {
					return nil, fmt.Errorf("field %s: functions (bytelen/count) are not allowed in array/buffer length expressions", field.Name)
				}
			}

			// Validate valueof: integer target, valid functions, existing
			// referenced fields. Record references for cycle detection.
			if meta.valueofExpr != "" {
				if meta.isArray {
					return nil, fmt.Errorf("field %s: valueof is not allowed on array/slice fields", field.Name)
				}
				switch meta.naturalType.iKind() {
				case intKind, uintKind, bitmapKind:
				default:
					return nil, fmt.Errorf("field %s: valueof requires an integer/bitmap field type, got %s", field.Name, meta.naturalType)
				}
				refs, fns, errRef := exprReferences(meta.valueofExpr)
				if errRef != nil {
					return nil, fmt.Errorf("field %s: invalid valueof expression: %w", field.Name, errRef)
				}
				for _, fn := range fns {
					if fn.name != "bytelen" && fn.name != "count" {
						return nil, fmt.Errorf("field %s: unknown function %s() in valueof (use bytelen or count)", field.Name, fn.name)
					}
					// count() is element count, valid only for slices/arrays.
					// Strings have no unambiguous element count under text
					// encodings — use bytelen for a string's byte length.
					if fn.name == "count" {
						if sf, ok := structType.FieldByName(fn.arg); ok {
							ft := sf.Type
							for ft.Kind() == reflect.Ptr {
								ft = ft.Elem()
							}
							if ft.Kind() != reflect.Slice && ft.Kind() != reflect.Array {
								return nil, fmt.Errorf("field %s: count(%s) requires a slice or array field (use bytelen for a string's byte length)", field.Name, fn.arg)
							}
						}
					}
				}
				for _, r := range refs {
					if _, ok := structType.FieldByName(r); !ok {
						return nil, fmt.Errorf("field %s: valueof references unknown field %s", field.Name, r)
					}
				}
				valueofRefs[field.Name] = refs
			}
		}

		fields = append(fields, meta)
	}

	// Reject reference cycles among valueof fields (e.g. A's valueof references
	// B and B's valueof references A), which would make encode-time evaluation
	// non-terminating.
	if len(valueofRefs) > 0 {
		valueofSet := make(map[string]bool, len(valueofRefs))
		for name := range valueofRefs {
			valueofSet[name] = true
		}
		var visit func(name string, path map[string]bool) error
		visit = func(name string, path map[string]bool) error {
			if path[name] {
				return fmt.Errorf("valueof reference cycle detected at field %s", name)
			}
			path[name] = true
			for _, r := range valueofRefs[name] {
				if valueofSet[r] {
					if err := visit(r, path); err != nil {
						return err
					}
				}
			}
			delete(path, name)
			return nil
		}
		for name := range valueofRefs {
			if err := visit(name, map[string]bool{}); err != nil {
				return nil, err
			}
		}
	}

	meta := &structMetadata{fields: fields}
	structMetadataCache.Store(structType, meta)
	return meta, nil
}
