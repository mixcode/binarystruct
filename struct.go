// Copyright 2021 github.com/mixcode

package binarystruct

import (
	"encoding/hex"
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
	arrayLenExpr  string   // outermost dimension expression (mirrors arrayDimExprs[0]), kept for back-compat
	arrayDimExprs []string // all dimension expressions for a multidimensional tag (`[4][N]int8` → ["4","N"])
	bufLenExpr    string
	// arrayLenConst/bufLenConst are true when the expression is a compile-time
	// constant (no field references) and was pre-resolved into option.arrayLen /
	// option.bufLen at metadata time. The encode/decode paths then skip
	// re-tokenizing and re-evaluating it per operation.
	arrayLenConst bool
	bufLenConst   bool
	valueofExpr   string
	// valueofCustom* hold a custom valueof evaluator parsed from a
	// `valueof=NAME(field, ...)` tag whose NAME is not a built-in (bytelen,
	// count). Empty name means the valueof (if any) is a built-in/arithmetic
	// expression handled by evalValueof. The evaluator is looked up by name on
	// the Marshaler at run time (not validated at parse time, since metadata is
	// cached per type while evaluators are registered per Marshaler).
	valueofCustomName string
	valueofCustomArgs []string
	encoding          string
	endian            endianOverride
	codec             string
	ignore            bool
	unexported        bool
	fieldErr          error
	omittable         bool
	omittableExpr     string
	naturalType       eType
	option            typeOption
	hasRange          bool
	rangeMin          float64
	rangeMax          float64
	hasRangeMin       bool
	hasRangeMax       bool
	hasMatch          bool
	matchPattern      string
	matchRegexp       *regexp.Regexp
	hasConst          bool
	constExpr         string // raw const= text, kept for codegen and error messages
	constIsBytes      bool   // target is a byte sequence (vs an integer/bitmap)
	constInt          int64  // integer target: the constant value to emit/validate
	constBytes        []byte // byte-sequence target: the constant bytes to emit/validate
}

type structMetadata struct {
	fields []structFieldMetadata
	// endian is the struct-level byte-order declaration, from a blank `_ struct{}`
	// sentinel field's `binary:"endian=…"` tag or inherited from a value-embedded
	// struct. endianNone when the struct declares no order of its own.
	endian endianOverride
	// defaultEncoding is the struct-level default text encoding, from the sentinel's
	// `binary:"encoding=…"` (or inherited via embedding). It is baked into each
	// string field's metadata that does not set its own encoding=, so it sits
	// between a per-field encoding= and the Marshaler's DefaultTextEncoding.
	defaultEncoding string
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

	// regexp to match a tag. Group 1 is the (possibly multi-dimensional) array
	// bracket run "[4][2]"; group 2 is the binary type; group 4 is the (buflen).
	mTag = regexp.MustCompile(`^\s*((?:\[[^\]]*\])*)([^\s\(\)\[\]]*)(\(([^\)]+)\))?`)

	// splits an array bracket run "[4][2]" into its per-dimension expressions.
	mTagDim = regexp.MustCompile(`\[([^\]]*)\]`)

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
	tokComma
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
		if c == ',' {
			tokens = append(tokens, token{tokComma, ","})
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
	// callFunc resolves a function call such as bytelen(Name) or count(Items),
	// or a custom multi-argument evaluator such as CRC32(Type, Data). When nil,
	// function calls are rejected (the case for decode-side size expressions,
	// where functions are not permitted).
	callFunc func(funcName string, args []string) (int, error)
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
		// function call: IDENT '(' IDENT (',' IDENT)* ')'
		if p.peek().typ == tokLParen {
			p.consume() // '('
			var args []string
			arg := p.consume()
			if arg.typ != tokIdent {
				return 0, fmt.Errorf("function %s() expects field-name arguments", t.val)
			}
			args = append(args, arg.val)
			for p.peek().typ == tokComma {
				p.consume() // ','
				a := p.consume()
				if a.typ != tokIdent {
					return 0, fmt.Errorf("function %s() expects field-name arguments", t.val)
				}
				args = append(args, a.val)
			}
			if p.consume().typ != tokRParen {
				return 0, fmt.Errorf("missing closing parenthesis in %s(...)", t.val)
			}
			if p.callFunc == nil {
				return 0, fmt.Errorf("function %s() is not allowed here (functions are valid only in valueof)", t.val)
			}
			return p.callFunc(t.val, args)
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

// evalConstIntExpr evaluates a constant integer expression (literals in
// decimal/hex/octal/binary, the operators + - * /, and parentheses). Field
// references and functions are rejected, so the result depends only on the
// expression text. Used by range bounds so they accept the same numeric syntax
// as size expressions (e.g. range=0x04034b50..0x04034b50).
func evalConstIntExpr(stmt string) (int, error) {
	tokens, err := tokenize(stmt)
	if err != nil {
		return 0, err
	}
	p := &tagParser{tokens: tokens} // resolveIdent/callFunc nil: constants only
	value, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.peek().typ != tokEOF {
		return 0, fmt.Errorf("unexpected token at end of expression: %s", p.peek().val)
	}
	return value, nil
}

// parseRangeBound parses a single range bound. It first tries a constant
// integer expression (so hex/octal/binary literals and arithmetic work); if
// that does not apply (e.g. a floating-point bound such as 1.5 or 1e3), it
// falls back to a plain float parse. Range values are stored as float64, so
// integer bounds beyond 2^53 lose precision — out of scope for range, which is
// why exact magic-number matching uses const= rather than range=N..N.
func parseRangeBound(s string) (float64, error) {
	if v, err := evalConstIntExpr(s); err == nil {
		return float64(v), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// parseConstHexBytes decodes a byte-sequence const value, which must be a hex
// blob such as 0x504b0304 (the bytes in natural order, endianness-independent).
// Underscores are allowed as digit separators.
func parseConstHexBytes(s string) ([]byte, error) {
	t := strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	if !strings.HasPrefix(t, "0x") && !strings.HasPrefix(t, "0X") {
		return nil, fmt.Errorf("byte-sequence const must be a hex blob like 0x504b0304, got %q", s)
	}
	h := t[2:]
	if len(h) == 0 || len(h)%2 != 0 {
		return nil, fmt.Errorf("byte-sequence const %q must have an even number of hex digits", s)
	}
	b := make([]byte, len(h)/2)
	if _, err := hex.Decode(b, []byte(h)); err != nil {
		return nil, fmt.Errorf("invalid hex in const %q: %w", s, err)
	}
	return b, nil
}

// resolveConst validates a const= option and precomputes the value to emit and
// validate. The target is either an integer/bitmap field (the const is an
// integer expression, written honoring the field's byte order) or a raw
// byte-sequence field [N]byte / string(N) (the const is a fixed-length hex blob
// written in natural order). goType is the field's Go type, used to read a fixed
// array length when the tag omits an explicit size.
func resolveConst(meta *structFieldMetadata, goType reflect.Type) error {
	et := meta.naturalType
	if meta.encodeType != Any && meta.encodeType != iInvalid {
		et = meta.encodeType
	}
	isBytes := et == String || (meta.isArray && (et == Byte || et == Uint8 || et == Int8))

	if !isBytes {
		if meta.isArray {
			return fmt.Errorf("field %s: const on an array requires a byte element type ([N]byte)", meta.name)
		}
		switch et.iKind() {
		case intKind, uintKind, bitmapKind:
		default:
			return fmt.Errorf("field %s: const requires an integer/bitmap or raw byte-sequence field, got %s", meta.name, et)
		}
		v, err := evalConstIntExpr(meta.constExpr)
		if err != nil {
			return fmt.Errorf("field %s: invalid const value %q: %w", meta.name, meta.constExpr, err)
		}
		meta.constIsBytes = false
		meta.constInt = int64(v)
		return nil
	}

	// Byte-sequence target.
	if meta.encoding != "" {
		return fmt.Errorf("field %s: const cannot be combined with encoding= (raw bytes only)", meta.name)
	}
	b, err := parseConstHexBytes(meta.constExpr)
	if err != nil {
		return fmt.Errorf("field %s: %w", meta.name, err)
	}
	// Determine the field's fixed byte length.
	n := -1
	switch {
	case meta.isArray && meta.arrayLenExpr != "":
		ln, errLen := evalConstIntExpr(meta.arrayLenExpr)
		if errLen != nil {
			return fmt.Errorf("field %s: const requires a constant array length", meta.name)
		}
		n = ln
	case !meta.isArray && meta.bufLenExpr != "":
		ln, errLen := evalConstIntExpr(meta.bufLenExpr)
		if errLen != nil {
			return fmt.Errorf("field %s: const requires a constant string size", meta.name)
		}
		n = ln
	case goType.Kind() == reflect.Array:
		n = goType.Len()
	}
	if n < 0 {
		return fmt.Errorf("field %s: const on a byte-sequence field requires a fixed size ([N]byte or string(N)) matching the %d-byte constant", meta.name, len(b))
	}
	if n != len(b) {
		return fmt.Errorf("field %s: const has %d bytes but the field size is %d", meta.name, len(b), n)
	}
	meta.constIsBytes = true
	meta.constBytes = b
	return nil
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
	name string   // "bytelen", "count", or a custom evaluator name
	args []string // referenced field names
}

// parseSingleCustomCall recognizes an expression that is exactly one function
// call of the form NAME(field, field, ...), returning the function name and its
// field-name arguments. ok is false for anything else — arithmetic, multiple
// calls, or bare references — so a custom valueof evaluator is required to be
// the entire expression (it cannot be combined with arithmetic in this version).
func parseSingleCustomCall(expr string) (name string, args []string, ok bool) {
	toks, err := tokenize(expr)
	if err != nil {
		return "", nil, false
	}
	i := 0
	next := func() token {
		if i < len(toks) {
			t := toks[i]
			i++
			return t
		}
		return token{tokEOF, ""}
	}
	t := next()
	if t.typ != tokIdent {
		return "", nil, false
	}
	name = t.val
	if next().typ != tokLParen {
		return "", nil, false
	}
	a := next()
	if a.typ != tokIdent {
		return "", nil, false
	}
	args = append(args, a.val)
	for {
		t := next()
		if t.typ == tokComma {
			a := next()
			if a.typ != tokIdent {
				return "", nil, false
			}
			args = append(args, a.val)
		} else if t.typ == tokRParen {
			break
		} else {
			return "", nil, false
		}
	}
	if next().typ != tokEOF {
		return "", nil, false
	}
	return name, args, true
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
		callFunc: func(fn string, args []string) (int, error) {
			funcs = append(funcs, exprFuncCall{name: fn, args: args})
			refs = append(refs, args...)
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

// splitTagOptions splits a binary-tag option list on commas, ignoring commas
// nested inside parentheses. This keeps a multi-argument valueof evaluator such
// as `CRC32(Type, Data)` as a single option, while the common single-arg forms
// (`string(StrLen+2)`, `bytelen(Name)`) split exactly as a plain comma split
// would (no existing tag carries a comma inside parentheses, so this is
// backward-compatible).
func splitTagOptions(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// parseArrayDims splits an array bracket run such as "[4][2]" into its
// per-dimension expression strings (["4","2"]); "[]" yields one empty entry, and
// an empty run yields nil (not an array).
func parseArrayDims(bracketRun string) []string {
	if bracketRun == "" {
		return nil
	}
	ms := mTagDim.FindAllStringSubmatch(bracketRun, -1)
	dims := make([]string, 0, len(ms))
	for _, m := range ms {
		dims = append(dims, strings.TrimSpace(m[1]))
	}
	return dims
}

// parse tag string directly
func parseTagString(tagStr string, strc reflect.Value, naturalType eType, naturalOption typeOption, fieldErr error) (encodeType eType, option typeOption, err error) {
	encodeType = naturalType
	option = naturalOption

	// read the tag
	tags := splitTagOptions(tagStr)
	if len(tags) == 0 || tags[0] == "" {
		// no tags to process
		if fieldErr != nil {
			err = fieldErr
		}
		return
	}

	m := mTag.FindStringSubmatch(tags[0])
	typeTag := m[2]
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

	// check for array type and its size(s); a run like [4][2] is multidimensional.
	dims := parseArrayDims(m[1])
	option.isArray = len(dims) > 0
	if option.isArray {
		option.dims = make([]int, len(dims))
		for i, d := range dims {
			if d == "" {
				continue // length comes from the value's own length (e.g. []byte)
			}
			var dv int
			dv, err = evaluateTagValue(strc, d)
			if err != nil {
				return
			}
			if dv < 0 {
				err = errNegativeSize
				return
			}
			option.dims[i] = dv
		}
		option.arrayLen = option.dims[0]
	}

	if m[4] != "" {
		option.bufLen, err = evaluateTagValue(strc, m[4])
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
		case "codec":
			if len(t) > 1 {
				option.codec = t[1]
			} else {
				err = fmt.Errorf("missing value for codec tag")
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

// parseEndianValue maps a tag's endian= value to an endianOverride.
func parseEndianValue(s string) (endianOverride, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "big":
		return endianBig, nil
	case "little":
		return endianLittle, nil
	case "inverse":
		return endianInverse, nil
	default:
		return endianNone, fmt.Errorf("unknown endian value: %s", s)
	}
}

// parseStructSentinel parses the struct-scope options carried by a blank
// `_ struct{}` sentinel field's binary tag: endian= (the struct's byte order) and
// encoding= (its default text encoding).
func parseStructSentinel(tagStr string) (eo endianOverride, encoding string, err error) {
	eo = endianNone
	for _, seg := range splitTagOptions(tagStr) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		kv := strings.SplitN(seg, "=", 2)
		key := strings.TrimSpace(kv[0])
		switch key {
		case "endian":
			if len(kv) < 2 {
				return endianNone, "", fmt.Errorf("missing value for endian in struct-level `_` sentinel tag")
			}
			e, perr := parseEndianValue(kv[1])
			if perr != nil {
				return endianNone, "", perr
			}
			eo = e
		case "encoding":
			if len(kv) < 2 || strings.TrimSpace(kv[1]) == "" {
				return endianNone, "", fmt.Errorf("missing value for encoding in struct-level `_` sentinel tag")
			}
			encoding = strings.TrimSpace(kv[1])
		default:
			return endianNone, "", fmt.Errorf("unknown struct-level option %q in `_` sentinel tag (only endian= and encoding= are supported)", key)
		}
	}
	return eo, encoding, nil
}

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

	// Struct-level byte order / default encoding: own* come from a blank `_`
	// sentinel field; inherited* from value-embedded structs that declare them.
	ownEndian := endianNone
	var inheritedEndians []endianOverride
	ownEncoding := ""
	var inheritedEncodings []string

	for i := 0; i < nField; i++ {
		field := structType.Field(i)
		fType := field.Type
		fKind := fType.Kind()

		// A blank `_ struct{}` field carries struct-scope options (endian=) and is
		// excluded from the layout — it is metadata, not an encoded field.
		if field.Name == "_" && fKind == reflect.Struct && fType.NumField() == 0 {
			if tagStr := field.Tag.Get(tagName); tagStr != "" {
				eo, enc, err := parseStructSentinel(tagStr)
				if err != nil {
					return nil, err
				}
				if eo != endianNone {
					ownEndian = eo
				}
				if enc != "" {
					ownEncoding = enc
				}
			}
			continue
		}

		// Guard a botched sentinel: a blank `_` field whose tag *starts* with a
		// struct-scope option (endian=/encoding=) but whose type is not struct{}.
		// It would otherwise silently drop the option (it isn't recognized as a
		// sentinel) and encode the field as data — a silent wrong-output bug. A
		// blank `_ <type>` reserved/padding field with a normal type tag (e.g.
		// `binary:"uint32"`) is fine and is not flagged.
		if field.Name == "_" {
			if tagStr := field.Tag.Get(tagName); tagStr != "" {
				first := strings.TrimSpace(strings.SplitN(tagStr, ",", 2)[0])
				if strings.HasPrefix(first, "endian=") || strings.HasPrefix(first, "encoding=") {
					return nil, fmt.Errorf("struct-level options (endian=/encoding=) must be on a blank `_ struct{}` field, but field %d is `_ %s`; change its type to struct{}", i, fType)
				}
			}
		}

		// A value-embedded struct that declares its own order propagates it to this
		// struct (D4). Pointer-embedded structs are skipped to avoid metadata cycles
		// (value embedding cannot form a cycle, so the recursion always terminates).
		if field.Anonymous && fKind == reflect.Struct {
			em, err := getStructMetadata(fType)
			if err != nil {
				return nil, err
			}
			if em.endian != endianNone {
				inheritedEndians = append(inheritedEndians, em.endian)
			}
			if em.defaultEncoding != "" {
				inheritedEncodings = append(inheritedEncodings, em.defaultEncoding)
			}
			// fall through: the embedded struct is still encoded as a nested field.
		}

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
		tags := splitTagOptions(tagStr)

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
		typeTag := m[2]
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

		dims := parseArrayDims(m[1])
		meta.isArray = len(dims) > 0
		if meta.isArray {
			meta.arrayDimExprs = dims
			meta.arrayLenExpr = dims[0] // outermost, for back-compat
		}

		if m[4] != "" {
			meta.bufLenExpr = m[4]
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
			case "codec":
				if len(t) > 1 {
					meta.codec = t[1]
				} else {
					return nil, fmt.Errorf("missing value for codec tag on field %s", field.Name)
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
			case "const":
				if len(t) > 1 {
					meta.hasConst = true
					meta.constExpr = strings.Join(t[1:], "=")
				} else {
					return nil, fmt.Errorf("missing value for const tag on field %s", field.Name)
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
						minVal, errParse := parseRangeBound(minStr)
						if errParse != nil {
							return nil, fmt.Errorf("invalid range min value on field %s: %w", field.Name, errParse)
						}
						meta.rangeMin = minVal
						meta.hasRangeMin = true
					}
					if maxStr != "" {
						maxVal, errParse := parseRangeBound(maxStr)
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
						meta.arrayLenConst = true
					}
				}
				// Pre-resolve constant array dimensions (field-referenced dims stay
				// 0 here and are resolved at encode/decode time). Multidimensional
				// only; 1-D keeps using arrayLen.
				if len(meta.arrayDimExprs) > 1 {
					meta.option.dims = make([]int, len(meta.arrayDimExprs))
					for i, d := range meta.arrayDimExprs {
						if d == "" {
							continue
						}
						if val, err := evaluateTagValue(reflect.Value{}, d); err == nil {
							meta.option.dims[i] = val
						}
					}
				}
			}
			if meta.bufLenExpr != "" {
				if val, err := evaluateTagValue(reflect.Value{}, meta.bufLenExpr); err == nil {
					meta.option.bufLen = val
					meta.bufLenConst = true
				}
			}
			if meta.encoding != "" {
				meta.option.encoding = meta.encoding
			}
			if meta.endian != endianNone {
				meta.option.endian = meta.endian
			}
			if meta.codec != "" {
				meta.option.codec = meta.codec
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
				// A function name that is not a built-in marks a custom
				// evaluator. It is dispatched at run time against the Marshaler's
				// registry (not validated here — see valueofCustom* docs).
				custom := false
				for _, fn := range fns {
					if fn.name != "bytelen" && fn.name != "count" {
						custom = true
						break
					}
				}
				if custom {
					name, args, isCall := parseSingleCustomCall(meta.valueofExpr)
					if !isCall {
						return nil, fmt.Errorf("field %s: a custom valueof evaluator must be the entire expression, e.g. valueof=%s(Field, ...); it cannot be combined with arithmetic or other functions", field.Name, fns[0].name)
					}
					meta.valueofCustomName = name
					meta.valueofCustomArgs = args
				} else {
					for _, fn := range fns {
						if len(fn.args) != 1 {
							return nil, fmt.Errorf("field %s: %s() takes exactly one field-name argument", field.Name, fn.name)
						}
						// count() is element count, valid only for slices/arrays.
						// Strings have no unambiguous element count under text
						// encodings — use bytelen for a string's byte length.
						if fn.name == "count" {
							if sf, ok := structType.FieldByName(fn.args[0]); ok {
								ft := sf.Type
								for ft.Kind() == reflect.Ptr {
									ft = ft.Elem()
								}
								if ft.Kind() != reflect.Slice && ft.Kind() != reflect.Array {
									return nil, fmt.Errorf("field %s: count(%s) requires a slice or array field (use bytelen for a string's byte length)", field.Name, fn.args[0])
								}
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

			// Validate and resolve const: emit-on-encode + validate-on-decode
			// of a fixed value. Target is an integer/bitmap or a raw byte
			// sequence ([N]byte / string(N)); the byte form uses a hex blob.
			if meta.hasConst {
				if meta.valueofExpr != "" {
					return nil, fmt.Errorf("field %s: const cannot be combined with valueof", field.Name)
				}
				if err := resolveConst(&meta, field.Type); err != nil {
					return nil, err
				}
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

	// Resolve the struct-level byte order: the struct's own `_` sentinel wins;
	// otherwise a single distinct order inherited from value-embedded structs.
	// Conflicting inherited orders are an error (V4) rather than a silent choice.
	structEndian := ownEndian
	if structEndian == endianNone {
		for _, e := range inheritedEndians {
			if structEndian == endianNone {
				structEndian = e
			} else if e != structEndian {
				return nil, fmt.Errorf("conflicting struct-level byte order inherited from embedded structs in %s", structType.Name())
			}
		}
	}

	// Resolve the struct-level default text encoding, same precedence as endian:
	// the struct's own sentinel wins; otherwise a single distinct inherited value;
	// conflicting inherited encodings are an error.
	structEncoding := ownEncoding
	if structEncoding == "" {
		for _, e := range inheritedEncodings {
			if structEncoding == "" {
				structEncoding = e
			} else if e != structEncoding {
				return nil, fmt.Errorf("conflicting struct-level default encoding inherited from embedded structs in %s", structType.Name())
			}
		}
	}

	// Bake the struct default encoding into string fields that declare none of
	// their own, so every execution path picks it up via fMeta.encoding. Skip const
	// fields (a byte-sequence const must not carry an encoding) and non-string
	// fields (encoding is meaningless for them).
	if structEncoding != "" {
		for i := range fields {
			f := &fields[i]
			if f.encoding == "" && !f.hasConst && f.encodeType.iKind() == stringKind {
				f.encoding = structEncoding
				f.option.encoding = structEncoding
			}
		}
	}

	meta := &structMetadata{fields: fields, endian: structEndian, defaultEncoding: structEncoding}
	structMetadataCache.Store(structType, meta)
	return meta, nil
}
