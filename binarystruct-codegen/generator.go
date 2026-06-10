// Copyright 2026 github.com/mixcode

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Generator struct {
	Dir          string
	Types        []string
	IncludeTests bool

	// Endian is the byte-order expression (e.g. "binarystruct.BigEndian") baked
	// into the no-arg MarshalBinary/UnmarshalBinary/AppendBinary methods, which
	// implement the order-less stdlib encoding interfaces. Set from the -endian
	// flag; required when generating Go code.
	Endian string

	// NoValidate, when true, strips ALL decode-time validation from the generated
	// read methods: const/range/match checks and custom valueof recompute-and-compare.
	// Default false = the generated decode validates everything, matching the runtime
	// interpreter exactly (full parity); set -no-validate to skip the checks for
	// trusted-input / hot-path decoding. Encode emission is unaffected (const still
	// writes its magic, valueof still computes its value). Set from the -no-validate flag.
	NoValidate bool

	// UnsafeBulk, when true, emits a raw-memory bulk path for fixed-width scalar
	// arrays/slices whose Go element width matches the wire width: a single
	// Write/ReadFull over the element backing store via unsafe, plus one in-place
	// binarystruct.SwapBytes when the order differs from the host (SIMD-accelerated
	// under -tags experiment_simd on amd64). Default false = the portable
	// per-element order.PutUintN/UintN bulk path (no unsafe import). The two paths
	// are byte-identical; this only trades portability for speed. Set from -unsafe-bulk.
	UnsafeBulk bool

	// structs holds every struct type parsed from the target package, keyed by
	// name. Populated by Generate; used to recognize nested-struct fields when
	// translating bytelen() (case 5).
	structs map[string]*ast.StructType
}

type parsedFieldTag struct {
	hasTag        bool
	binaryType    string
	isArray       bool
	arrayLenExpr  string
	bufLenExpr    string
	options       map[string]string
	numDims       int      // number of array dimensions; >1 is a multidimensional tag
	arrayDimExprs []string // per-dimension length expressions for a multidimensional tag
}

// Group 1 is the (possibly multi-dimensional) array bracket run "[4][2]"; group 2
// is the binary type; group 4 is the (buflen). mTagDim splits the run per-dimension.
var mTag = regexp.MustCompile(`^\s*((?:\[[^\]]*\])*)([^\s\(\)\[\]]*)(\(([^\)]+)\))?`)
var mTagDim = regexp.MustCompile(`\[([^\]]*)\]`)

func parseFieldTag(tag *ast.BasicLit) parsedFieldTag {
	res := parsedFieldTag{options: make(map[string]string)}
	if tag == nil {
		return res
	}
	tagVal, err := strconv.Unquote(tag.Value)
	if err != nil {
		return res
	}
	re := regexp.MustCompile(`binary:"([^"]*)"`)
	m := re.FindStringSubmatch(tagVal)
	if len(m) < 2 {
		return res
	}
	res.hasTag = true
	tagStr, err := strconv.Unquote("\"" + m[1] + "\"")
	if err != nil {
		tagStr = m[1]
	}

	tags := splitTagOptions(tagStr)
	if len(tags) == 0 || tags[0] == "" {
		return res
	}

	match := mTag.FindStringSubmatch(tags[0])
	if len(match) >= 5 {
		dims := mTagDim.FindAllStringSubmatch(match[1], -1)
		res.isArray = len(dims) > 0
		res.numDims = len(dims)
		if res.isArray {
			res.arrayDimExprs = make([]string, len(dims))
			for i, d := range dims {
				res.arrayDimExprs[i] = strings.TrimSpace(d[1])
			}
			res.arrayLenExpr = res.arrayDimExprs[0] // outermost dimension
		}
		res.binaryType = match[2]
		res.bufLenExpr = match[4]
	}

	for _, opt := range tags[1:] {
		parts := strings.SplitN(opt, "=", 2)
		val := ""
		if len(parts) > 1 {
			val = parts[1]
		}
		res.options[parts[0]] = val
	}

	return res
}

// structSentinelEndian scans a struct's fields for a blank `_` sentinel carrying
// a struct-level endian= and returns the byte-order literal expression
// ("binarystruct.BigEndian"/"binarystruct.LittleEndian"), or "" if none is
// declared. endian=inverse is unsupported by codegen (it depends on a runtime
// caller order), so it is a generation-time error.
func structSentinelEndian(st *ast.StructType) (string, error) {
	binRe := regexp.MustCompile(`binary:"([^"]*)"`)
	for _, field := range st.Fields.List {
		if len(field.Names) != 1 || field.Names[0].Name != "_" || field.Tag == nil {
			continue
		}
		tagVal, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		m := binRe.FindStringSubmatch(tagVal)
		if len(m) < 2 {
			continue
		}
		for _, seg := range strings.Split(m[1], ",") {
			kv := strings.SplitN(strings.TrimSpace(seg), "=", 2)
			if strings.TrimSpace(kv[0]) != "endian" {
				continue
			}
			if len(kv) < 2 {
				return "", fmt.Errorf("missing value for endian in `_` sentinel tag")
			}
			switch strings.ToLower(strings.TrimSpace(kv[1])) {
			case "big":
				return "binarystruct.BigEndian", nil
			case "little":
				return "binarystruct.LittleEndian", nil
			case "inverse":
				return "", fmt.Errorf("struct-level endian=inverse is not supported by codegen; use the runtime interpreter")
			default:
				return "", fmt.Errorf("unknown endian value %q in `_` sentinel tag", kv[1])
			}
		}
	}
	return "", nil
}

// structSentinelEncoding returns the struct-level default text encoding declared
// on a blank `_` sentinel field (`binary:"encoding=NAME"`), or "" if none.
func structSentinelEncoding(st *ast.StructType) string {
	binRe := regexp.MustCompile(`binary:"([^"]*)"`)
	for _, field := range st.Fields.List {
		if len(field.Names) != 1 || field.Names[0].Name != "_" || field.Tag == nil {
			continue
		}
		tagVal, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		m := binRe.FindStringSubmatch(tagVal)
		if len(m) < 2 {
			continue
		}
		for _, seg := range strings.Split(m[1], ",") {
			kv := strings.SplitN(strings.TrimSpace(seg), "=", 2)
			if strings.TrimSpace(kv[0]) == "encoding" && len(kv) == 2 {
				return strings.TrimSpace(kv[1])
			}
		}
	}
	return ""
}

// isStringBinType reports whether a binary type name is a text-string family type
// (the kinds to which a text encoding applies).
func isStringBinType(binType string) bool {
	switch binType {
	case "string", "bstring", "wstring", "dwstring", "zstring", "z16string":
		return true
	}
	return false
}

// applyStructEncoding bakes the struct-level default text encoding into a string
// field's parsed tag when the field declares no encoding= of its own — mirroring
// the runtime, which bakes it into the field metadata. It mutates pt.options (a
// reference), so downstream EncodeText/DecodeText/bytelen emitters pick it up.
func applyStructEncoding(pt parsedFieldTag, structEnc string) {
	if structEnc != "" && isStringBinType(pt.binaryType) && pt.options["encoding"] == "" {
		pt.options["encoding"] = structEnc
	}
}

func getGoTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return getGoTypeName(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + getGoTypeName(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + getGoTypeName(t.Elt)
		}
		var buf bytes.Buffer
		format.Node(&buf, token.NewFileSet(), t.Len)
		return "[" + buf.String() + "]" + getGoTypeName(t.Elt)
	default:
		return ""
	}
}

func translateExpression(expr string) string {
	if expr == "" {
		return ""
	}
	re := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_\.]*`)
	return re.ReplaceAllStringFunc(expr, func(s string) string {
		if s == "true" || s == "false" || s == "nil" {
			return s
		}
		return "s." + s
	})
}

type cgFieldInfo struct {
	goType      string
	encoding    string
	hasValueof  bool
	valueofExpr string

	// Layout metadata used to compute bytelen() of this field at generation time
	// (cases 2/3/5). binType is the effective binary type; arrayLenExpr/bufLenExpr
	// are the [N] and (N) tag sub-expressions; isStruct is true when the field's
	// (deref/element) Go type is a struct defined in the same package.
	binType      string
	isArray      bool
	arrayLenExpr string
	bufLenExpr   string
	isStruct     bool
}

var (
	cgValueofFuncRe = regexp.MustCompile(`s\.(bytelen|count)\(s\.([a-zA-Z_][a-zA-Z0-9_]*)\)`)
	cgIdentRe       = regexp.MustCompile(`s\.([a-zA-Z_][a-zA-Z0-9_]*)`)
	// cgValueofCallRe matches a function call's name in a raw valueof expression
	// (an identifier immediately followed by '('), used to reject custom
	// evaluators the generator cannot resolve statically.
	cgValueofCallRe = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	cgPlainIdentRe  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	cgConstIntRe    = regexp.MustCompile(`^\s*(0[xX][0-9a-fA-F_]+|[0-9][0-9_]*)\s*$`)
)

// parseCustomValueofCall recognizes an expression that is exactly one function
// call NAME(field, field, ...), returning the name and field-name arguments. ok
// is false for anything else (arithmetic, multiple calls, bare references), so a
// custom valueof evaluator must be the whole expression. Mirrors the runtime
// parseSingleCustomCall in the parent package's struct.go.
func parseCustomValueofCall(expr string) (name string, args []string, ok bool) {
	expr = strings.TrimSpace(expr)
	open := strings.IndexByte(expr, '(')
	if open <= 0 || !strings.HasSuffix(expr, ")") {
		return "", nil, false
	}
	name = strings.TrimSpace(expr[:open])
	if !cgPlainIdentRe.MatchString(name) {
		return "", nil, false
	}
	inner := expr[open+1 : len(expr)-1]
	for _, a := range splitTagOptions(inner) {
		a = strings.TrimSpace(a)
		if !cgPlainIdentRe.MatchString(a) {
			return "", nil, false
		}
		args = append(args, a)
	}
	if len(args) == 0 {
		return "", nil, false
	}
	return name, args, true
}

// cgValueofArgBytesExpr returns a Go expression yielding the encoded []byte of a
// custom-valueof argument field, plus any hoisted pre-statements. Byte regions
// ([]byte/[N]byte at natural length, raw string, constant-size string(N)) and
// fixed-width integer scalars are emitted inline (no Marshaler, honoring the
// runtime `order`). Every other shape — text-encoded/prefixed strings, floats,
// multibyte-scalar arrays, padded byte slices, variable string buffers — is
// re-encoded with its own tag via ms.MarshalAs (the runtime encoder, so the bytes
// match fieldEncodedBytes exactly). A nested struct stays unsupported (its byte
// order can't be expressed in a standalone tag) and fails generation. endianStr
// ("big"/"little") is the struct's resolved order, baked into the MarshalAs tag.
func cgValueofArgBytesExpr(name string, fi cgFieldInfo, endianStr string) (expr, pre string, err error) {
	gt := fi.goType
	// Static fast paths: shapes whose encoded bytes the generator reproduces inline.
	switch {
	case strings.HasPrefix(gt, "[]") && isByteSequence(gt):
		// A byte slice encodes to its own bytes only at its natural length; a
		// constant fixed length pads/truncates — handled by the MarshalAs path below.
		if !(fi.arrayLenExpr != "" && cgConstIntRe.MatchString(fi.arrayLenExpr)) {
			return "s." + name, "", nil
		}
	case strings.HasPrefix(gt, "[") && isByteSequence(gt):
		// Go fixed array [N]byte: always exactly N bytes.
		return "s." + name + "[:]", "", nil
	case fi.binType == "string" && fi.encoding == "":
		if fi.bufLenExpr == "" {
			return "[]byte(s." + name + ")", "", nil
		}
		if cgConstIntRe.MatchString(fi.bufLenExpr) {
			bufVar := "vo" + name
			return bufVar, fmt.Sprintf("\t%s := make([]byte, %s)\n\tcopy(%s, []byte(s.%s))\n", bufVar, strings.TrimSpace(fi.bufLenExpr), bufVar, name), nil
		}
		// variable-size string buffer → MarshalAs path below.
	}
	// Fixed-width integer scalars: exact wire bytes via order.PutUintN.
	if !fi.isArray {
		v := "vo" + name
		switch fi.binType {
		case "int8", "uint8", "byte":
			return fmt.Sprintf("[]byte{byte(s.%s)}", name), "", nil
		case "int16", "uint16", "word":
			return v, fmt.Sprintf("\t%s := make([]byte, 2)\n\torder.PutUint16(%s, uint16(s.%s))\n", v, v, name), nil
		case "int32", "uint32", "dword":
			return v, fmt.Sprintf("\t%s := make([]byte, 4)\n\torder.PutUint32(%s, uint32(s.%s))\n", v, v, name), nil
		case "int64", "uint64", "qword":
			return v, fmt.Sprintf("\t%s := make([]byte, 8)\n\torder.PutUint64(%s, uint64(s.%s))\n", v, v, name), nil
		}
	}
	// Hard shapes: re-encode the single arg via the runtime encoder.
	if fi.isStruct {
		return "", "", fmt.Errorf("codegen custom valueof cannot encode nested-struct argument %q standalone; use the runtime interpreter for this struct", name)
	}
	tag := cgMarshalAsTag(fi, endianStr)
	v := "vo" + name
	pre = fmt.Sprintf("\t%s, errVo := ms.MarshalAs(s.%s, %s)\n\tif errVo != nil {\n\t\treturn n, errVo\n\t}\n", v, name, strconv.Quote(tag))
	return v, pre, nil
}

// cgMarshalAsTag reconstructs the binary tag for a custom-valueof argument field
// so ms.MarshalAs re-encodes it byte-identically to the runtime's fieldEncodedBytes.
// Field-referenced lengths are dropped (MarshalAs gets only the standalone value,
// and the runtime measures such fields at their natural length anyway); constant
// lengths are kept (they pad/truncate). The struct's resolved byte order is baked
// in so multibyte content (length prefixes, floats, multibyte scalars) matches.
func cgMarshalAsTag(fi cgFieldInfo, endianStr string) string {
	var b strings.Builder
	switch {
	case fi.isArray:
		b.WriteString("[")
		if al := strings.TrimSpace(fi.arrayLenExpr); cgConstIntRe.MatchString(al) {
			b.WriteString(al)
		}
		b.WriteString("]")
		b.WriteString(fi.binType)
	case fi.bufLenExpr != "" && cgConstIntRe.MatchString(strings.TrimSpace(fi.bufLenExpr)):
		b.WriteString(fi.binType)
		b.WriteString("(")
		b.WriteString(strings.TrimSpace(fi.bufLenExpr))
		b.WriteString(")")
	default:
		b.WriteString(fi.binType)
	}
	if fi.encoding != "" {
		b.WriteString(",encoding=")
		b.WriteString(fi.encoding)
	}
	if endianStr != "" {
		b.WriteString(",endian=")
		b.WriteString(endianStr)
	}
	return b.String()
}

// splitTagOptions splits a binary-tag option list on commas, ignoring commas
// nested inside parentheses. Mirrors the runtime parser so a multi-argument
// valueof such as CRC32(Type, Data) stays a single option (backward-compatible:
// no existing tag carries a comma inside parentheses).
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

// isByteSequence reports whether goType is a byte-like slice or array, whose
// element count equals its encoded byte length.
func isByteSequence(goType string) bool {
	elem := goType
	if strings.HasPrefix(elem, "[") {
		if i := strings.IndexByte(elem, ']'); i >= 0 {
			elem = elem[i+1:]
		}
	}
	return elem == "byte" || elem == "uint8" || elem == "int8"
}

// stripToElemType removes pointer, slice and fixed-array prefixes from a Go type
// name, yielding the underlying element type (e.g. "[]*Header" -> "Header").
func stripToElemType(goType string) string {
	for {
		switch {
		case strings.HasPrefix(goType, "*"):
			goType = goType[1:]
		case strings.HasPrefix(goType, "[]"):
			goType = goType[2:]
		case strings.HasPrefix(goType, "["):
			if i := strings.IndexByte(goType, ']'); i >= 0 {
				goType = goType[i+1:]
				continue
			}
			return goType
		default:
			return goType
		}
	}
}

// isStructType reports whether goType (after stripping pointer/slice/array
// prefixes) names a struct defined in the target package.
func (g *Generator) isStructType(goType string) bool {
	_, ok := g.structs[stripToElemType(goType)]
	return ok
}

// isGeneratedType reports whether goType (after stripping pointer/slice/array
// prefixes) names a type this run is generating methods for — i.e. it will have
// its own WriteBinaryWithMarshaler/ReadBinaryWithMarshaler. Nested fields of such
// types are encoded by a direct generated-method call instead of the runtime
// reflection interpreter.
func (g *Generator) isGeneratedType(goType string) bool {
	name := stripToElemType(goType)
	for _, t := range g.Types {
		if t == name {
			return true
		}
	}
	return false
}

// scalarWidth returns the fixed encoded byte width of a scalar binary type, and
// whether binType is such a fixed-width scalar. Used to compute bytelen() of
// scalar and scalar-array fields statically (case 2).
func scalarWidth(binType string) (int, bool) {
	switch binType {
	case "int8", "uint8", "byte":
		return 1, true
	case "int16", "uint16", "word":
		return 2, true
	case "int32", "uint32", "dword", "float32":
		return 4, true
	case "int64", "uint64", "qword", "float64":
		return 8, true
	}
	return 0, false
}

// stringPrefixWidth returns the byte width of a length-prefixed string's prefix
// (0 for non-prefixed forms), matching the encode path in generateFieldWrite.
func stringPrefixWidth(binType string) int {
	switch binType {
	case "bstring":
		return 1
	case "wstring":
		return 2
	case "dwstring":
		return 4
	}
	return 0
}

// stringTermWidth returns the byte width of a null-terminated string's
// terminator (0 for non-terminated forms).
func stringTermWidth(binType string) int {
	switch binType {
	case "zstring":
		return 1
	case "z16string":
		return 2
	}
	return 0
}

// translateValueof converts a valueof expression into a Go integer expression.
// It returns any hoisted pre-statements (measurement blocks emitted before the
// length field) alongside the expression itself; the caller must write `pre`
// immediately before the field write. count() and bytelen() of byte sequences /
// raw strings become len(s.F); fixed-width scalars and scalar arrays become
// width*count; fixed strings string(N) become N; a plain nested struct is
// measured at runtime via a hoisted binarystruct.Write(io.Discard, ...). Cases
// that cannot be computed byte-exactly from static type info (text-encoded
// variable strings, slice/array of structs, a reference to another valueof
// field) return a generation-time error rather than emitting wrong code.
func (g *Generator) translateValueof(expr string, fields map[string]cgFieldInfo, visiting map[string]bool) (pre string, out string, err error) {
	// Custom valueof evaluators are registered on a Marshaler at run time and
	// cannot be embedded in standalone generated code. Reject them with a clear
	// message (like endian=inverse and embedding-inherited order) so the struct
	// falls back to the runtime interpreter rather than emitting a call to a
	// nonexistent method.
	for _, fm := range cgValueofCallRe.FindAllStringSubmatch(expr, -1) {
		if fm[1] != "bytelen" && fm[1] != "count" {
			return "", "", fmt.Errorf("codegen does not support custom valueof evaluators (%s()); use the runtime interpreter for this struct", fm[1])
		}
	}
	prefixed := translateExpression(expr) // e.g. s.bytelen(s.Name)+2
	var ferr error
	var preBuf bytes.Buffer
	measured := make(map[string]bool) // dedup hoisted measurements by field name
	out = cgValueofFuncRe.ReplaceAllStringFunc(prefixed, func(m string) string {
		sub := cgValueofFuncRe.FindStringSubmatch(m)
		fn, arg := sub[1], sub[2]
		fi, ok := fields[arg]
		if !ok {
			ferr = fmt.Errorf("valueof references unknown field %q", arg)
			return m
		}
		switch fn {
		case "count":
			// count() is element count, valid only for slices/arrays.
			if strings.HasPrefix(fi.goType, "[") {
				return fmt.Sprintf("len(s.%s)", arg)
			}
			ferr = fmt.Errorf("count(%s) requires a slice or array field (got %q); use bytelen for a string's byte length", arg, fi.goType)
			return m
		case "bytelen":
			repl, p, e := g.bytelenExpr(arg, fi, fields, measured, visiting)
			if e != nil {
				ferr = e
				return m
			}
			preBuf.WriteString(p)
			return repl
		}
		return m
	})
	if ferr != nil {
		return "", "", ferr
	}
	// A bare reference to another valueof field cannot be resolved in generated
	// code (it would read the field's pre-encode value, diverging from runtime).
	for _, sm := range cgIdentRe.FindAllStringSubmatch(out, -1) {
		if fi, ok := fields[sm[1]]; ok && fi.hasValueof {
			return "", "", fmt.Errorf("codegen does not support a valueof expression referencing another valueof field (%q); use the runtime interpreter", sm[1])
		}
	}
	return preBuf.String(), out, nil
}

// bytelenExpr computes the Go expression (and any hoisted pre-statements) for the
// encoded byte length of field `arg`. See translateValueof for the supported
// cases. `measured` deduplicates runtime measurement temps when a field's
// bytelen() appears more than once in a single expression.
func (g *Generator) bytelenExpr(arg string, fi cgFieldInfo, fields map[string]cgFieldInfo, measured map[string]bool, visiting map[string]bool) (expr, pre string, err error) {
	// case 1: byte sequences -> element count equals byte count.
	if isByteSequence(fi.goType) {
		return fmt.Sprintf("len(s.%s)", arg), "", nil
	}

	// string forms (string / bstring / wstring / dwstring / zstring / z16string).
	// Encoded size = prefix width + content length + terminator width, mirroring
	// the encode path. Pointer-to-string is not handled here (rare).
	if fi.goType == "string" {
		extra := stringPrefixWidth(fi.binType) + stringTermWidth(fi.binType)
		addExtra := func(base string) string {
			if extra == 0 {
				return base
			}
			return fmt.Sprintf("(%s + %d)", base, extra)
		}

		switch {
		case fi.bufLenExpr != "":
			// case 3: buffered string(N) -> content is exactly N bytes (padded or
			// truncated), regardless of encoding.
			bufSize, e := g.translateEncodeExpr(fi.bufLenExpr, fields, visiting)
			if e != nil {
				return "", "", e
			}
			return addExtra("int(" + bufSize + ")"), "", nil
		case fi.encoding == "":
			// case 1: raw, unbounded, unencoded content -> len().
			return addExtra(fmt.Sprintf("len(s.%s)", arg)), "", nil
		default:
			// case 4: unbounded text-encoded content -> measure with the same
			// ms-guarded EncodeText the write path uses (raw when ms is nil).
			tmp := "bl" + arg
			if measured[arg] {
				return addExtra(tmp), "", nil
			}
			measured[arg] = true
			inner := tmp + "Bytes"
			pre = fmt.Sprintf("\tvar %s int\n\t{\n\t\t%s := []byte(s.%s)\n\t\tif ms != nil {\n\t\t\t%s, err = ms.EncodeText(%s, %q)\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t}\n\t\t%s = len(%s)\n\t}\n", tmp, inner, arg, inner, inner, fi.encoding, tmp, inner)
			return addExtra(tmp), pre, nil
		}
	}

	// case 5: a nested struct -> measure at runtime using the identical call(s)
	// the write path uses, guaranteeing a byte-exact result.
	if fi.isStruct {
		isPtr := strings.HasPrefix(fi.goType, "*")
		if isPtr && fi.isArray {
			return "", "", fmt.Errorf("codegen does not support bytelen(%s) of a pointer-element struct array; use the runtime interpreter for this struct", arg)
		}
		tmp := "bl" + arg
		if measured[arg] {
			return tmp, "", nil
		}
		measured[arg] = true
		if isPtr {
			// A nil pointer encodes to zero bytes; mirror the write's nil guard.
			// Passing the pointer matches the write's binarystruct.Write(&*ptr).
			pre = fmt.Sprintf("\tvar %s int\n\tif s.%s != nil {\n\t\t%s, err = binarystruct.NewMarshalerOrder(order).Write(io.Discard, s.%s)\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n", tmp, arg, tmp, arg)
			return tmp, pre, nil
		}
		if fi.isArray {
			// A tag-counted array of structs ([N]Elem). Mirror the encode loop:
			// same element count and the same per-element binarystruct.Write.
			sizeExpr, e := g.translateEncodeExpr(fi.arrayLenExpr, fields, visiting)
			if e != nil {
				return "", "", e
			}
			if sizeExpr == "" {
				sizeExpr = fmt.Sprintf("len(s.%s)", arg)
			}
			pre = fmt.Sprintf("\tvar %s int\n\t{\n\t\tlimit := int(%s)\n\t\tfor i := 0; i < limit; i++ {\n\t\t\tvar mm int\n\t\t\tmm, err = binarystruct.NewMarshalerOrder(order).Write(io.Discard, &s.%s[i])\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t\t%s += mm\n\t\t}\n\t}\n", tmp, sizeExpr, arg, tmp)
			return tmp, pre, nil
		}
		// A single struct value, or a Go-native slice/array of structs written as
		// a whole value: the write path encodes it with one binarystruct.Write.
		pre = fmt.Sprintf("\tvar %s int\n\t%s, err = binarystruct.NewMarshalerOrder(order).Write(io.Discard, &s.%s)\n\tif err != nil {\n\t\treturn n, err\n\t}\n", tmp, tmp, arg)
		return tmp, pre, nil
	}

	// case 2: fixed-width scalar or scalar array/slice -> width * count.
	if w, ok := scalarWidth(fi.binType); ok {
		if strings.HasPrefix(fi.goType, "*") {
			// A pointer scalar may be nil (0 bytes); a constant width would be wrong.
			return "", "", fmt.Errorf("codegen does not support bytelen(%s) of a pointer scalar field; use the runtime interpreter for this struct", arg)
		}
		if fi.isArray {
			sizeExpr, e := g.translateEncodeExpr(fi.arrayLenExpr, fields, visiting)
			if e != nil {
				return "", "", e
			}
			if sizeExpr == "" {
				sizeExpr = fmt.Sprintf("len(s.%s)", arg)
			}
			return fmt.Sprintf("((%s) * %d)", sizeExpr, w), "", nil
		}
		// single scalar field has a constant width.
		return strconv.Itoa(w), "", nil
	}

	return "", "", fmt.Errorf("codegen does not support bytelen(%s) of type %q (e.g. struct slice/array); use the runtime interpreter for this struct", arg, fi.goType)
}

// translateEncodeExpr translates a decode-side size expression for use on the
// ENCODE path: any referenced valueof field is replaced by its computed
// expression (so e.g. [NameLen]byte writes len(s.Name) bytes rather than the
// stale s.NameLen). Non-valueof references become s.Field as usual.
func (g *Generator) translateEncodeExpr(expr string, fields map[string]cgFieldInfo, visiting map[string]bool) (string, error) {
	if expr == "" {
		return "", nil
	}
	prefixed := translateExpression(expr)
	var ferr error
	out := cgIdentRe.ReplaceAllStringFunc(prefixed, func(m string) string {
		name := cgIdentRe.FindStringSubmatch(m)[1]
		fi, ok := fields[name]
		if ok && fi.hasValueof {
			// Guard a self-referential cycle: this size expression references a
			// valueof field whose own value is already being resolved up the stack
			// (e.g. valueof=bytelen(F) where F is string(thatVeryField)). Without
			// this, translateValueof <-> translateEncodeExpr recurse until the stack
			// overflows. Emit a clean error instead.
			if visiting[name] {
				ferr = fmt.Errorf("codegen does not support a self-referential valueof/bytelen cycle through field %q (e.g. valueof=bytelen(F) where F is string(%s)); use the runtime interpreter for this struct", name, name)
				return m
			}
			visiting[name] = true
			pre, sub, err := g.translateValueof(fi.valueofExpr, fields, visiting)
			delete(visiting, name)
			if err != nil {
				ferr = err
				return m
			}
			// A size expression is spliced inline; it cannot host the hoisted
			// measurement statements a struct-valued bytelen() would require.
			if pre != "" {
				ferr = fmt.Errorf("codegen cannot inline a size expression that references valueof %q (its bytelen() needs a runtime measurement); use the runtime interpreter for this struct", name)
				return m
			}
			return "(" + sub + ")"
		}
		return m
	})
	if ferr != nil {
		return "", ferr
	}
	return out, nil
}

func (g *Generator) Generate(outPath string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, g.Dir, func(fi os.FileInfo) bool {
		if fi.Name() == filepath.Base(outPath) {
			return false
		}
		if strings.HasSuffix(fi.Name(), "_test.go") {
			return g.IncludeTests
		}
		return true
	}, 0)
	if err != nil {
		return fmt.Errorf("failed to parse directory: %w", err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no Go packages found in %s", g.Dir)
	}

	var pkg *ast.Package
	var pkgName string
	for name, p := range pkgs {
		pkg = p
		pkgName = name
		break
	}

	structs := make(map[string]*ast.StructType)
	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			structs[ts.Name.Name] = st
			return true
		})
	}
	g.structs = structs

	var buf bytes.Buffer
	buf.WriteString("// Code generated by binarystruct-codegen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", pkgName)

	needMath := false
	needRegexp := false
	needErrors := false
	needFmt := false
	needUnsafe := false

	for _, typeName := range g.Types {
		st, ok := structs[typeName]
		if !ok {
			return fmt.Errorf("type %s not found in package %s", typeName, pkgName)
		}
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 || field.Names[0].Name == "_" {
				continue
			}
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)
			binType := getEffectiveBinaryType(parsedTag.binaryType, goType)

			// float fields need math for the bits<->float conversion, EXCEPT when a
			// float slice takes the raw-memory bulk path (which copies bytes directly
			// and never calls math.FloatNbits).
			if binType == "float32" || binType == "float64" {
				_, bulkUnsafe := g.cgArrayCanBulkUnsafe(goType, binType, parsedTag)
				if !(parsedTag.isArray && parsedTag.numDims <= 1 && bulkUnsafe) {
					needMath = true
				}
			}
			// const/range/match decode validation is emitted unless -no-validate.
			if _, ok := parsedTag.options["match"]; ok && !g.NoValidate {
				needRegexp = true
				needFmt = true
			}
			if _, ok := parsedTag.options["range"]; ok && !g.NoValidate {
				needFmt = true
			}
			if cexpr, ok := parsedTag.options["const"]; ok && cexpr != "" && !g.NoValidate {
				needFmt = true
			}
			if val, ok := parsedTag.options["codec"]; ok && val != "" {
				needErrors = true
				needFmt = true
			}
			// A custom valueof evaluator emits an ms-nil guard (errors) and an
			// unknown-evaluator / mismatch error (fmt), like a codec.
			if vexpr, ok := parsedTag.options["valueof"]; ok && vexpr != "" {
				if evname, _, isCall := parseCustomValueofCall(vexpr); isCall && evname != "bytelen" && evname != "count" {
					needErrors = true
					needFmt = true
				}
			}
			if parsedTag.isArray && parsedTag.arrayLenExpr == "" {
				needErrors = true
			}
			// A scalar array/slice whose Go element width matches the wire width
			// uses the raw-memory bulk path, which references unsafe.
			if parsedTag.isArray && parsedTag.numDims <= 1 {
				if _, ok := g.cgArrayCanBulkUnsafe(goType, binType, parsedTag); ok {
					needUnsafe = true
				}
			}
		}
	}

	buf.WriteString("import (\n")
	buf.WriteString("\t\"bytes\"\n")
	if needErrors {
		buf.WriteString("\t\"errors\"\n")
	}
	if needFmt {
		buf.WriteString("\t\"fmt\"\n")
	}
	buf.WriteString("\t\"io\"\n")
	if needMath {
		buf.WriteString("\t\"math\"\n")
	}
	if needRegexp {
		buf.WriteString("\t\"regexp\"\n")
	}
	if needUnsafe {
		buf.WriteString("\t\"unsafe\"\n")
	}
	buf.WriteString("\t\"github.com/mixcode/binarystruct\"\n")
	buf.WriteString(")\n\n")

	// Generate regex variables for match checks
	for _, typeName := range g.Types {
		st, ok := structs[typeName]
		if !ok {
			return fmt.Errorf("type %s not found in package %s", typeName, pkgName)
		}
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 || field.Names[0].Name == "_" {
				continue
			}
			fieldName := field.Names[0].Name
			parsedTag := parseFieldTag(field.Tag)
			if pattern, ok := parsedTag.options["match"]; ok && !g.NoValidate {
				fmt.Fprintf(&buf, "var regex_%s_%s = regexp.MustCompile(`%s`)\n", typeName, fieldName, pattern)
			}
		}
	}
	buf.WriteString("\n")

	for _, typeName := range g.Types {
		st := structs[typeName]
		if err := g.generateMethods(&buf, typeName, st); err != nil {
			return err
		}
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Dump unformatted code for debugging
		ioutil.WriteFile(outPath, buf.Bytes(), 0644)
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	return ioutil.WriteFile(outPath, formatted, 0644)
}

var (
	reUsesTmp = regexp.MustCompile(`\btmp\b`)
	reUsesM   = regexp.MustCompile(`\bm\b`)
)

// emitLocalScratch writes the `tmp`/`m` scratch declarations a generated method
// needs, but only when its body actually references them. A body that uses
// neither (e.g. a struct whose sole field is an unbounded string, decoded via
// io.ReadAll) would otherwise fail to compile with "declared and not used".
func emitLocalScratch(buf *bytes.Buffer, body string) {
	if reUsesTmp.MatchString(body) {
		buf.WriteString("\tvar tmp [8]byte\n")
	}
	if reUsesM.MatchString(body) {
		buf.WriteString("\tvar m int\n")
	}
}

// baseTypeName strips pointer/slice/array wrappers from a Go type expression,
// returning the underlying identifier (e.g. "[]*Record" -> "Record", "[4]T" -> "T").
func baseTypeName(goType string) string {
	s := goType
	for {
		switch {
		case strings.HasPrefix(s, "*"):
			s = s[1:]
		case strings.HasPrefix(s, "[]"):
			s = s[2:]
		case strings.HasPrefix(s, "["):
			i := strings.IndexByte(s, ']')
			if i < 0 {
				return s
			}
			s = s[i+1:]
		default:
			return s
		}
	}
}

// orderedParentHint makes the "no byte order" error actionable for a nested type
// processed in isolation: it looks for another parsed struct that has a field of
// type typeName AND declares its own byte order (the order typeName would inherit
// at runtime), and returns a clause naming it. Returns "" if none is found.
// Struct names are scanned in sorted order so the message is deterministic.
func (g *Generator) orderedParentHint(typeName string) string {
	names := make([]string, 0, len(g.structs))
	for name := range g.structs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == typeName {
			continue
		}
		lit, err := structSentinelEndian(g.structs[name])
		if err != nil || lit == "" {
			continue // this struct declares no order of its own
		}
		for _, field := range g.structs[name].Fields.List {
			if len(field.Names) == 0 || field.Names[0].Name == "_" {
				continue
			}
			if baseTypeName(getGoTypeName(field.Type)) == typeName {
				word := "big"
				if lit == "binarystruct.LittleEndian" {
					word = "little"
				}
				return fmt.Sprintf(" (it is used by %s, which is %s-endian; pass `-endian %s`, or declare the order on %s itself)", name, word, word, typeName)
			}
		}
	}
	return ""
}

// emittableFields returns the struct's fields that produce code (named, non-`_`).
func emittableFields(st *ast.StructType) []*ast.Field {
	var out []*ast.Field
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || f.Names[0].Name == "_" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// cgFieldBatchable reports a field's wire width if it can join a contiguous
// scalar-field batch (a run of ≥2 is coalesced into one shared buffer + a single
// Write/ReadFull, replacing per-field Write/ReadFull — byte-identical, fewer
// io calls). Batchable = a plain fixed-width scalar with no option that needs
// per-field handling (valueof/const/codec/omittable/ignore/array/pointer or a
// per-field endian/encoding override). Conservative by design.
func cgFieldBatchable(f *ast.Field, structEnc string) (int, bool) {
	pt := parseFieldTag(f.Tag)
	applyStructEncoding(pt, structEnc)
	if pt.binaryType == "-" || pt.isArray {
		return 0, false
	}
	// Exclude anything needing per-field handling: emit-only computed values
	// (valueof/const), decode-time validation (const/range/match — the batch read
	// skips it), custom codecs, omission, ignored fields, and per-field
	// endian/encoding overrides.
	for _, opt := range []string{"ignore", "omittable", "valueof", "const", "range", "match", "codec", "encoding", "endian"} {
		if _, has := pt.options[opt]; has {
			return 0, false
		}
	}
	goType := getGoTypeName(f.Type)
	if strings.HasPrefix(goType, "*") {
		return 0, false
	}
	return scalarWidth(getEffectiveBinaryType(pt.binaryType, goType))
}

// scalarRunEnd returns the index just past the maximal run of batchable scalar
// fields starting at i.
func scalarRunEnd(flds []*ast.Field, i int, structEnc string) int {
	j := i
	for j < len(flds) {
		if _, ok := cgFieldBatchable(flds[j], structEnc); !ok {
			break
		}
		j++
	}
	return j
}

// batchTotalWidth sums the wire widths of a batchable run.
func batchTotalWidth(flds []*ast.Field, structEnc string) int {
	total := 0
	for _, f := range flds {
		w, _ := cgFieldBatchable(f, structEnc)
		total += w
	}
	return total
}

// generateScalarFieldBatchWrite emits one shared buffer filled by per-field
// order.PutUintN at fixed offsets, then a single w.Write — replacing N per-field
// PutUintN+Write pairs. Byte-identical to the per-field path.
func (g *Generator) generateScalarFieldBatchWrite(buf *bytes.Buffer, flds []*ast.Field, structEnc string) {
	fmt.Fprintf(buf, "\t{\n\t\tsbuf := make([]byte, %d)\n", batchTotalWidth(flds, structEnc))
	off := 0
	for _, f := range flds {
		w, _ := cgFieldBatchable(f, structEnc)
		goType := getGoTypeName(f.Type)
		binType := getEffectiveBinaryType(parseFieldTag(f.Tag).binaryType, goType)
		acc := "s." + f.Names[0].Name
		switch {
		case w == 1:
			fmt.Fprintf(buf, "\t\tsbuf[%d] = byte(%s)\n", off, acc)
		case binType == "float32":
			fmt.Fprintf(buf, "\t\torder.PutUint32(sbuf[%d:%d], math.Float32bits(float32(%s)))\n", off, off+4, acc)
		case binType == "float64":
			fmt.Fprintf(buf, "\t\torder.PutUint64(sbuf[%d:%d], math.Float64bits(float64(%s)))\n", off, off+8, acc)
		case w == 2:
			fmt.Fprintf(buf, "\t\torder.PutUint16(sbuf[%d:%d], uint16(%s))\n", off, off+2, acc)
		case w == 4:
			fmt.Fprintf(buf, "\t\torder.PutUint32(sbuf[%d:%d], uint32(%s))\n", off, off+4, acc)
		case w == 8:
			fmt.Fprintf(buf, "\t\torder.PutUint64(sbuf[%d:%d], uint64(%s))\n", off, off+8, acc)
		}
		off += w
	}
	buf.WriteString("\t\tm, err = w.Write(sbuf)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
}

// generateScalarFieldBatchRead emits a single io.ReadFull into a shared buffer,
// then per-field order.UintN decode at fixed offsets — replacing N per-field reads.
func (g *Generator) generateScalarFieldBatchRead(buf *bytes.Buffer, flds []*ast.Field, structEnc string) {
	fmt.Fprintf(buf, "\t{\n\t\tsbuf := make([]byte, %d)\n", batchTotalWidth(flds, structEnc))
	buf.WriteString("\t\tm, err = io.ReadFull(r, sbuf)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
	off := 0
	for _, f := range flds {
		w, _ := cgFieldBatchable(f, structEnc)
		goType := getGoTypeName(f.Type)
		binType := getEffectiveBinaryType(parseFieldTag(f.Tag).binaryType, goType)
		dst := "s." + f.Names[0].Name
		switch {
		case w == 1:
			fmt.Fprintf(buf, "\t\t%s = %s(sbuf[%d])\n", dst, goType, off)
		case binType == "float32":
			fmt.Fprintf(buf, "\t\t%s = %s(math.Float32frombits(order.Uint32(sbuf[%d:%d])))\n", dst, goType, off, off+4)
		case binType == "float64":
			fmt.Fprintf(buf, "\t\t%s = %s(math.Float64frombits(order.Uint64(sbuf[%d:%d])))\n", dst, goType, off, off+8)
		case w == 2:
			fmt.Fprintf(buf, "\t\t%s = %s(order.Uint16(sbuf[%d:%d]))\n", dst, goType, off, off+2)
		case w == 4:
			fmt.Fprintf(buf, "\t\t%s = %s(order.Uint32(sbuf[%d:%d]))\n", dst, goType, off, off+4)
		case w == 8:
			fmt.Fprintf(buf, "\t\t%s = %s(order.Uint64(sbuf[%d:%d]))\n", dst, goType, off, off+8)
		}
		off += w
	}
	buf.WriteString("\t}\n")
}

func (g *Generator) generateMethods(buf *bytes.Buffer, typeName string, st *ast.StructType) error {
	// Resolve the type's byte order. A struct-level `_` sentinel declaration wins;
	// otherwise the -endian flag supplies the order baked into the no-arg stdlib
	// methods. If neither is present, generation fails (the stdlib encoding
	// interfaces carry no order, so we must not fabricate one).
	structLit, err := structSentinelEndian(st)
	if err != nil {
		return fmt.Errorf("type %s: %w", typeName, err)
	}
	bakedLit := structLit
	if bakedLit == "" {
		bakedLit = g.Endian
	}
	if bakedLit == "" {
		return fmt.Errorf("type %s: no byte order — declare it on the struct (a blank `_ struct{}` field tagged `binary:\"endian=big|little\"`) or pass -endian%s", typeName, g.orderedParentHint(typeName))
	}
	// The struct's resolved order as a tag word ("big"/"little"), baked into the
	// ms.MarshalAs tag for any hard custom-valueof argument (see cgMarshalAsTag).
	endianStr := ""
	switch bakedLit {
	case "binarystruct.BigEndian":
		endianStr = "big"
	case "binarystruct.LittleEndian":
		endianStr = "little"
	}

	// Struct-level default text encoding (the `_` sentinel's encoding=): a string
	// field that declares no encoding= of its own inherits it. Baked into each
	// field's parsed tag below (applyStructEncoding), mirroring the runtime.
	structEnc := structSentinelEncoding(st)

	// Multidimensional array tags ([4][2]int8) are supported for scalar leaves with
	// all-fixed or all-slice nesting; other shapes fail loud so the struct falls
	// back to the runtime interpreter (which supports every shape).
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 || field.Names[0].Name == "_" {
			continue
		}
		pt := parseFieldTag(field.Tag)
		if pt.numDims > 1 {
			goType := getGoTypeName(field.Type)
			binType := getEffectiveBinaryType(pt.binaryType, goType)
			if reason := cgMultidimUnsupported(goType, binType, pt); reason != "" {
				return fmt.Errorf("type %s: field %s: %s; use the runtime interpreter for this struct", typeName, field.Names[0].Name, reason)
			}
		}
	}

	// Write standard helper functions. The no-arg stdlib encoding interfaces carry
	// no byte order, so they bake bakedLit (the struct's declared order if any,
	// else the -endian flag).
	fmt.Fprintf(buf, "// MarshalBinary implements encoding.BinaryMarshaler.\n")
	fmt.Fprintf(buf, "func (s *%s) MarshalBinary() ([]byte, error) {\n", typeName)
	buf.WriteString("\tvar b bytes.Buffer\n")
	fmt.Fprintf(buf, "\t_, err := s.WriteBinary(&b, %s)\n", bakedLit)
	buf.WriteString("\treturn b.Bytes(), err\n")
	buf.WriteString("}\n\n")

	fmt.Fprintf(buf, "// AppendBinary implements encoding.BinaryAppender.\n")
	fmt.Fprintf(buf, "func (s *%s) AppendBinary(b []byte) ([]byte, error) {\n", typeName)
	buf.WriteString("\tbuf := bytes.NewBuffer(b)\n")
	fmt.Fprintf(buf, "\t_, err := s.WriteBinary(buf, %s)\n", bakedLit)
	buf.WriteString("\treturn buf.Bytes(), err\n")
	buf.WriteString("}\n\n")

	fmt.Fprintf(buf, "// UnmarshalBinary implements encoding.BinaryUnmarshaler.\n")
	fmt.Fprintf(buf, "func (s *%s) UnmarshalBinary(data []byte) error {\n", typeName)
	buf.WriteString("\tr := bytes.NewReader(data)\n")
	fmt.Fprintf(buf, "\t_, err := s.ReadBinary(r, %s)\n", bakedLit)
	buf.WriteString("\treturn err\n")
	buf.WriteString("}\n\n")

	// 1. WriteBinary (Standard)
	fmt.Fprintf(buf, "// WriteBinary implements binarystruct.BinaryWriter.\n")
	fmt.Fprintf(buf, "func (s *%s) WriteBinary(w io.Writer, order binarystruct.ByteOrder) (int, error) {\n", typeName)
	buf.WriteString("\treturn s.WriteBinaryWithMarshaler(nil, w, order)\n")
	buf.WriteString("}\n\n")

	// 2. WriteBinaryWithMarshaler (Context-aware)
	fmt.Fprintf(buf, "// WriteBinaryWithMarshaler implements binarystruct.MarshalerContextWriter.\n")
	fmt.Fprintf(buf, "func (s *%s) WriteBinaryWithMarshaler(ms *binarystruct.Marshaler, w io.Writer, order binarystruct.ByteOrder) (n int, err error) {\n", typeName)

	// Field info for resolving valueof's bytelen()/count() at generation time.
	fieldInfo := make(map[string]cgFieldInfo)
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 || field.Names[0].Name == "_" {
			continue
		}
		pt := parseFieldTag(field.Tag)
		applyStructEncoding(pt, structEnc)
		vexpr, hasV := pt.options["valueof"]
		goType := getGoTypeName(field.Type)
		fieldInfo[field.Names[0].Name] = cgFieldInfo{
			goType:       goType,
			encoding:     pt.options["encoding"],
			hasValueof:   hasV,
			valueofExpr:  vexpr,
			binType:      getEffectiveBinaryType(pt.binaryType, goType),
			isArray:      pt.isArray,
			arrayLenExpr: pt.arrayLenExpr,
			bufLenExpr:   pt.bufLenExpr,
			isStruct:     g.isStructType(goType),
		}
	}

	var writeBody bytes.Buffer
	if err := func() error {
		buf := &writeBody
		flds := emittableFields(st)
		for fi := 0; fi < len(flds); fi++ {
			if rj := scalarRunEnd(flds, fi, structEnc); rj-fi >= 2 {
				g.generateScalarFieldBatchWrite(buf, flds[fi:rj], structEnc)
				fi = rj - 1
				continue
			}
			field := flds[fi]
			fieldName := field.Names[0].Name
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)
			applyStructEncoding(parsedTag, structEnc)

			if _, ok := parsedTag.options["ignore"]; ok || parsedTag.binaryType == "-" {
				continue
			}

			// Handle omittable with expression
			if omittableExpr, ok := parsedTag.options["omittable"]; ok && omittableExpr != "" {
				fmt.Fprintf(buf, "\tif n >= int(%s) {\n\t\treturn n, nil\n\t}\n", translateExpression(omittableExpr))
			} else if ok && strings.HasPrefix(goType, "*") {
				// EOF-based omission (pointer)
				fmt.Fprintf(buf, "\tif s.%s == nil {\n\t\treturn n, nil\n\t}\n", fieldName)
			}

			binType := getEffectiveBinaryType(parsedTag.binaryType, goType)

			// valueof: write a value computed from other fields instead of the
			// field's own (emit-only). Validated as an integer scalar upstream.
			if vexpr, ok := parsedTag.options["valueof"]; ok && vexpr != "" {
				// A custom evaluator (a single NAME(...) call whose NAME is not a
				// built-in) is resolved on the Marshaler at run time, like a codec.
				if evname, cargs, isCall := parseCustomValueofCall(vexpr); isCall && evname != "bytelen" && evname != "count" {
					if err := g.generateCustomValueofWrite(buf, fieldName, evname, cargs, goType, binType, parsedTag, fieldInfo, endianStr); err != nil {
						return fmt.Errorf("field %s: %w", fieldName, err)
					}
					continue
				}
				pre, valExpr, vErr := g.translateValueof(vexpr, fieldInfo, map[string]bool{})
				if vErr != nil {
					return fmt.Errorf("field %s: %w", fieldName, vErr)
				}
				buf.WriteString(pre)
				if err := g.generateFieldWrite(buf, "("+valExpr+")", goType, binType, parsedTag, fieldInfo); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
				continue
			}

			// const: emit a fixed value (emit-only), ignoring the struct field.
			if cexpr, ok := parsedTag.options["const"]; ok && cexpr != "" {
				if err := g.generateConstWrite(buf, goType, binType, parsedTag, fieldInfo, cexpr); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
				continue
			}

			if parsedTag.isArray {
				if err := g.generateArrayWrite(buf, fieldName, goType, binType, parsedTag, fieldInfo); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
			} else {
				if err := g.generateFieldWrite(buf, "s."+fieldName, goType, binType, parsedTag, fieldInfo); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	if structLit != "" {
		// A struct-declared order wins over the order the caller passed in (the
		// runtime fast-paths here before seeding the struct order, so we seed it).
		fmt.Fprintf(buf, "\torder = %s\n", structLit)
	}
	emitLocalScratch(buf, writeBody.String())
	buf.Write(writeBody.Bytes())
	buf.WriteString("\treturn n, nil\n")
	buf.WriteString("}\n\n")

	// 3. ReadBinary (Standard)
	fmt.Fprintf(buf, "// ReadBinary implements binarystruct.BinaryReader.\n")
	fmt.Fprintf(buf, "func (s *%s) ReadBinary(r io.Reader, order binarystruct.ByteOrder) (int, error) {\n", typeName)
	buf.WriteString("\treturn s.ReadBinaryWithMarshaler(nil, r, order)\n")
	buf.WriteString("}\n\n")

	// 4. ReadBinaryWithMarshaler (Context-aware)
	fmt.Fprintf(buf, "// ReadBinaryWithMarshaler implements binarystruct.MarshalerContextReader.\n")
	fmt.Fprintf(buf, "func (s *%s) ReadBinaryWithMarshaler(ms *binarystruct.Marshaler, r io.Reader, order binarystruct.ByteOrder) (n int, err error) {\n", typeName)

	var readBody bytes.Buffer
	if err := func() error {
		buf := &readBody
		flds := emittableFields(st)
		for fi := 0; fi < len(flds); fi++ {
			if rj := scalarRunEnd(flds, fi, structEnc); rj-fi >= 2 {
				g.generateScalarFieldBatchRead(buf, flds[fi:rj], structEnc)
				fi = rj - 1
				continue
			}
			field := flds[fi]
			fieldName := field.Names[0].Name
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)
			applyStructEncoding(parsedTag, structEnc)

			if _, ok := parsedTag.options["ignore"]; ok || parsedTag.binaryType == "-" {
				continue
			}

			binType := getEffectiveBinaryType(parsedTag.binaryType, goType)

			// Handle omittable
			if omittableExpr, ok := parsedTag.options["omittable"]; ok {
				if omittableExpr != "" {
					fmt.Fprintf(buf, "\tif n >= int(%s) {\n\t\treturn n, nil\n\t}\n", translateExpression(omittableExpr))
				} else {
					// EOF-based omission
					buf.WriteString("\t// EOF check for omittable\n")
					buf.WriteString("\t{\n")
					buf.WriteString("\t\tvar peek [1]byte\n")
					buf.WriteString("\t\t_, peekErr := io.ReadFull(r, peek[:])\n")
					buf.WriteString("\t\tif peekErr == io.EOF || peekErr == io.ErrUnexpectedEOF {\n")
					buf.WriteString("\t\t\treturn n, nil\n")
					buf.WriteString("\t\t}\n")
					buf.WriteString("\t\t// Restore the byte\n")
					buf.WriteString("\t\tr = io.MultiReader(bytes.NewReader(peek[:]), r)\n")
					buf.WriteString("\t}\n")
				}
			}

			// Capture the field's start offset before reading it, so a validation
			// failure reports the same byte offset as the runtime interpreter
			// (which records n before advancing past the field). Only emitted for
			// fields that actually validate, to avoid an unused variable.
			offExpr := "n"
			if !g.NoValidate {
				_, hasConst := parsedTag.options["const"]
				_, hasRange := parsedTag.options["range"]
				_, hasMatch := parsedTag.options["match"]
				if hasConst || hasRange || hasMatch {
					offExpr = "voff" + fieldName
					fmt.Fprintf(buf, "\t%s := n\n", offExpr)
				}
			}

			if parsedTag.isArray {
				g.generateArrayRead(buf, fieldName, goType, binType, parsedTag, typeName, offExpr)
			} else {
				g.generateFieldRead(buf, "s."+fieldName, goType, binType, parsedTag, typeName, fieldName, offExpr)
			}

			// const: validate the field equals its fixed value after reading
			// (unless -no-validate strips decode validation).
			if cexpr, ok := parsedTag.options["const"]; ok && cexpr != "" && !g.NoValidate {
				if err := g.generateConstValidate(buf, fieldName, goType, binType, cexpr, offExpr); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
			}
		}
		// Post-decode validation of custom valueof evaluators. Run after all
		// fields are read so a checksum may reference fields declared after it.
		// Stripped by -no-validate, in which case the field was already read as a
		// plain scalar and is left unverified.
		if !g.NoValidate {
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 || field.Names[0].Name == "_" {
					continue
				}
				fieldName := field.Names[0].Name
				goType := getGoTypeName(field.Type)
				parsedTag := parseFieldTag(field.Tag)
				vexpr, ok := parsedTag.options["valueof"]
				if !ok || vexpr == "" {
					continue
				}
				evname, cargs, isCall := parseCustomValueofCall(vexpr)
				if !isCall || evname == "bytelen" || evname == "count" {
					continue
				}
				if err := g.generateCustomValueofValidate(buf, fieldName, evname, cargs, goType, fieldInfo, endianStr); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	if structLit != "" {
		fmt.Fprintf(buf, "\torder = %s\n", structLit)
	}
	emitLocalScratch(buf, readBody.String())
	buf.Write(readBody.Bytes())
	buf.WriteString("\treturn n, nil\n")
	buf.WriteString("}\n\n")

	return nil
}

// parseCgConstBytes decodes a byte-sequence const hex blob (e.g. 0x504b0304).
func parseCgConstBytes(s string) ([]byte, error) {
	t := strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	if !strings.HasPrefix(t, "0x") && !strings.HasPrefix(t, "0X") {
		return nil, fmt.Errorf("byte-sequence const must be a hex blob like 0x504b0304, got %q", s)
	}
	h := t[2:]
	if len(h) == 0 || len(h)%2 != 0 {
		return nil, fmt.Errorf("byte-sequence const %q must have an even number of hex digits", s)
	}
	out := make([]byte, len(h)/2)
	if _, err := hex.Decode(out, []byte(h)); err != nil {
		return nil, fmt.Errorf("invalid hex in const %q: %w", s, err)
	}
	return out, nil
}

// isFixedArrayType reports whether goType is a fixed-size array ([N]T) rather
// than a slice ([]T).
func isFixedArrayType(goType string) bool {
	return len(goType) > 1 && goType[0] == '[' && goType[1] != ']'
}

// isCgScalarBinType reports whether binType is a fixed-width scalar that
// generateFieldWrite/Read can emit as a multidimensional-array leaf.
func isCgScalarBinType(binType string) bool {
	switch binType {
	case "int8", "uint8", "byte", "int16", "uint16", "word",
		"int32", "uint32", "dword", "int64", "uint64", "qword",
		"float32", "float64":
		return true
	}
	return false
}

// cgArrayLevels peels numDims array levels off goType, reporting whether each
// level is a slice ([]) or a fixed array ([N]), plus the remaining leaf type. ok
// is false if goType has fewer array levels than the tag declares.
func cgArrayLevels(goType string, numDims int) (isSlice []bool, leaf string, ok bool) {
	s := goType
	for k := 0; k < numDims; k++ {
		switch {
		case strings.HasPrefix(s, "[]"):
			isSlice = append(isSlice, true)
			s = s[2:]
		case len(s) > 1 && s[0] == '[':
			idx := strings.IndexByte(s, ']')
			if idx < 0 {
				return nil, "", false
			}
			isSlice = append(isSlice, false)
			s = s[idx+1:]
		default:
			return nil, "", false
		}
	}
	return isSlice, s, true
}

// cgPeelArrayLevels returns the Go type of an element after k index operations
// (e.g. cgPeelArrayLevels("[][]int16", 1) == "[]int16"), used to size slice
// levels with make() on decode.
func cgPeelArrayLevels(goType string, k int) string {
	s := goType
	for ; k > 0; k-- {
		switch {
		case strings.HasPrefix(s, "[]"):
			s = s[2:]
		case len(s) > 0 && s[0] == '[':
			idx := strings.IndexByte(s, ']')
			if idx < 0 {
				return s
			}
			s = s[idx+1:]
		default:
			return s
		}
	}
	return s
}

// cgMultidimUnsupported returns a non-empty reason when codegen cannot emit a
// multidimensional array field, so generateMethods can fail loud (runtime
// fallback). Supported: a scalar leaf with all-fixed or all-slice nesting (slice
// levels need an explicit dimension for decode allocation).
func cgMultidimUnsupported(goType, binType string, pt parsedFieldTag) string {
	if !isCgScalarBinType(binType) {
		return fmt.Sprintf("codegen supports multidimensional arrays only over scalar leaf types, not %q", binType)
	}
	isSlice, leaf, ok := cgArrayLevels(goType, pt.numDims)
	if !ok || isFixedArrayType(leaf) || strings.HasPrefix(leaf, "[]") {
		return fmt.Sprintf("codegen could not match %d array dimensions to Go type %q", pt.numDims, goType)
	}
	for k, sl := range isSlice {
		if sl && (k >= len(pt.arrayDimExprs) || pt.arrayDimExprs[k] == "") {
			return fmt.Sprintf("a multidimensional slice needs an explicit length for dimension %d (for decode allocation)", k+1)
		}
	}
	return ""
}

// goByteSliceLiteral formats bytes as a Go []byte{...} literal.
func goByteSliceLiteral(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("0x%02x", x)
	}
	return "[]byte{" + strings.Join(parts, ", ") + "}"
}

// isCgBytesConst reports whether a const target is a raw byte sequence (vs an
// integer/bitmap scalar).
func isCgBytesConst(goType, binType string) bool {
	return binType == "string" || goType == "string" || isByteSequence(goType)
}

// generateConstWrite emits a fixed value on encode. Integer targets reuse the
// scalar writer (honoring endian); byte-sequence targets write a fixed []byte.
func (g *Generator) generateConstWrite(buf *bytes.Buffer, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo, cexpr string) error {
	if isCgBytesConst(goType, binType) {
		b, err := parseCgConstBytes(cexpr)
		if err != nil {
			return err
		}
		fmt.Fprintf(buf, "\tm, err = w.Write(%s)\n", goByteSliceLiteral(b))
		buf.WriteString("\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
		return nil
	}
	return g.generateFieldWrite(buf, "("+cexpr+")", goType, binType, parsedTag, fields)
}

// cgValidationErr formats a `return n, &binarystruct.DecodeError{...}` statement
// that mirrors the runtime interpreter's validation failures: a *DecodeError with
// the field's start offset (offExpr) and field name, wrapping an inner error that
// itself wraps ErrValidationError. inner is the Go expression for that inner error
// (e.g. `fmt.Errorf("const mismatch: %w", binarystruct.ErrValidationError)`).
func cgValidationErr(offExpr, fieldName, inner string) string {
	return fmt.Sprintf("\t\treturn n, &binarystruct.DecodeError{Offset: %s, Field: %q, Err: %s}\n", offExpr, fieldName, inner)
}

// generateConstValidate emits a post-read check that the field equals its const.
func (g *Generator) generateConstValidate(buf *bytes.Buffer, fieldName, goType, binType, cexpr, offExpr string) error {
	accessor := "s." + fieldName
	inner := `fmt.Errorf("const mismatch: %w", binarystruct.ErrValidationError)`
	if isCgBytesConst(goType, binType) {
		b, err := parseCgConstBytes(cexpr)
		if err != nil {
			return err
		}
		got := accessor
		switch {
		case goType == "string":
			got = "[]byte(" + accessor + ")"
		case strings.HasPrefix(goType, "["):
			got = accessor + "[:]"
		}
		fmt.Fprintf(buf, "\tif !bytes.Equal(%s, %s) {\n", got, goByteSliceLiteral(b))
		buf.WriteString(cgValidationErr(offExpr, fieldName, inner))
		buf.WriteString("\t}\n")
		return nil
	}
	fmt.Fprintf(buf, "\tif %s != (%s) {\n", accessor, cexpr)
	buf.WriteString(cgValidationErr(offExpr, fieldName, inner))
	buf.WriteString("\t}\n")
	return nil
}

// generateCustomValueofWrite emits the encode-time computation of a custom
// valueof evaluator: it builds a ValueOfContext from the referenced fields'
// encoded bytes, calls the evaluator looked up on the Marshaler by name, and
// writes the returned value through the normal scalar path. Like custom codecs,
// it requires a non-nil Marshaler (the no-arg MarshalBinary passes nil).
func (g *Generator) generateCustomValueofWrite(buf *bytes.Buffer, fieldName, evname string, args []string, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo, endianStr string) error {
	fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaler required for valueof %s\")\n\t}\n", evname)
	buf.WriteString("\t{\n")
	g.emitValueofLookup(buf, evname)
	argExprs, err := g.customValueofArgs(buf, evname, args, fields, endianStr)
	if err != nil {
		return err
	}
	buf.WriteString("\t\tvar voVal uint64\n")
	fmt.Fprintf(buf, "\t\tvoVal, err = fn(binarystruct.ValueOfContext{Struct: s, Target: %q, Args: []binarystruct.ValueOfArg{%s}})\n", fieldName, strings.Join(argExprs, ", "))
	buf.WriteString("\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
	if err := g.generateFieldWrite(buf, "voVal", goType, binType, parsedTag, fields); err != nil {
		return err
	}
	buf.WriteString("\t}\n")
	return nil
}

// generateCustomValueofValidate emits a post-decode check that recomputes the
// custom evaluator over the decoded fields and compares it to the value read
// from the stream, erroring (wrapping ErrValidationError) on mismatch. Emitted
// by default (parity with the runtime); -no-validate strips it, in which case the
// field is read as a plain scalar with no verification.
func (g *Generator) generateCustomValueofValidate(buf *bytes.Buffer, fieldName, evname string, args []string, goType string, fields map[string]cgFieldInfo, endianStr string) error {
	fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaler required for valueof %s\")\n\t}\n", evname)
	buf.WriteString("\t{\n")
	g.emitValueofLookup(buf, evname)
	argExprs, err := g.customValueofArgs(buf, evname, args, fields, endianStr)
	if err != nil {
		return err
	}
	buf.WriteString("\t\tvar voVal uint64\n")
	fmt.Fprintf(buf, "\t\tvoVal, err = fn(binarystruct.ValueOfContext{Struct: s, Target: %q, Decoding: true, Args: []binarystruct.ValueOfArg{%s}})\n", fieldName, strings.Join(argExprs, ", "))
	buf.WriteString("\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
	fmt.Fprintf(buf, "\t\tif s.%s != %s(voVal) {\n", fieldName, strings.TrimPrefix(goType, "*"))
	// Match the runtime's validateCustomValueofs: a *DecodeError whose Offset is
	// the end of the struct (n here) and whose Err wraps ErrValidationError, so
	// errors.As(&DecodeError) and errors.Is(ErrValidationError) both behave the
	// same whether decoded via the interpreter or generated code.
	fmt.Fprintf(buf, "\t\t\treturn n, &binarystruct.DecodeError{Offset: n, Field: %q, Err: fmt.Errorf(\"valueof %s() mismatch: %%w\", binarystruct.ErrValidationError)}\n", fieldName, evname)
	buf.WriteString("\t\t}\n")
	buf.WriteString("\t}\n")
	return nil
}

// emitValueofLookup writes the `fn := ms.GetValueOf(name)` lookup + nil guard,
// shared by the encode and decode-validate emitters (inside an open block).
func (g *Generator) emitValueofLookup(buf *bytes.Buffer, evname string) {
	fmt.Fprintf(buf, "\t\tfn := ms.GetValueOf(%q)\n", evname)
	buf.WriteString("\t\tif fn == nil {\n\t\t\treturn n, fmt.Errorf(\"unknown valueof evaluator: %s\", " + strconv.Quote(evname) + ")\n\t\t}\n")
}

// customValueofArgs emits hoisted pre-statements (deduped per referenced field)
// for the byte-region arguments and returns the []binarystruct.ValueOfArg literal
// elements (one per arg, in order).
func (g *Generator) customValueofArgs(buf *bytes.Buffer, evname string, args []string, fields map[string]cgFieldInfo, endianStr string) ([]string, error) {
	var argExprs []string
	seen := make(map[string]bool)
	for _, a := range args {
		fi, ok := fields[a]
		if !ok {
			return nil, fmt.Errorf("valueof %s() references unknown field %q", evname, a)
		}
		expr, pre, err := cgValueofArgBytesExpr(a, fi, endianStr)
		if err != nil {
			return nil, err
		}
		if !seen[a] {
			buf.WriteString(pre)
			seen[a] = true
		}
		argExprs = append(argExprs, fmt.Sprintf("{Name: %q, Bytes: %s, Value: s.%s}", a, expr, a))
	}
	return argExprs, nil
}

func (g *Generator) generateFieldWrite(buf *bytes.Buffer, target, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo) error {
	// Dereference pointer if necessary
	isPtr := strings.HasPrefix(goType, "*")
	accessor := target
	if isPtr {
		accessor = "*" + target
		fmt.Fprintf(buf, "\tif %s != nil {\n", target)
	}

	if val, ok := parsedTag.options["codec"]; ok && val != "" {
		fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaller context required for custom codec %s\")\n\t}\n", val)
		fmt.Fprintf(buf, "\t{\n\t\tser := ms.GetCodec(%q)\n", val)
		buf.WriteString("\t\tif ser == nil {\n\t\t\treturn n, fmt.Errorf(\"unknown codec: %s\", " + strconv.Quote(val) + ")\n\t\t}\n")
		fmt.Fprintf(buf, "\t\tm, err = ser.Encode(w, %s, nil, -1, order)\n", accessor)
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		if isPtr {
			buf.WriteString("\t}\n")
		}
		return nil
	}

	switch binType {
	case "int8", "uint8", "byte":
		fmt.Fprintf(buf, "\ttmp[0] = byte(%s)\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:1])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "int16", "uint16", "word":
		fmt.Fprintf(buf, "\torder.PutUint16(tmp[:2], uint16(%s))\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:2])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "int32", "uint32", "dword":
		fmt.Fprintf(buf, "\torder.PutUint32(tmp[:4], uint32(%s))\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:4])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "int64", "uint64", "qword":
		fmt.Fprintf(buf, "\torder.PutUint64(tmp[:8], uint64(%s))\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:8])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "float32":
		fmt.Fprintf(buf, "\torder.PutUint32(tmp[:4], math.Float32bits(float32(%s)))\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:4])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "float64":
		fmt.Fprintf(buf, "\torder.PutUint64(tmp[:8], math.Float64bits(float64(%s)))\n", accessor)
		buf.WriteString("\tm, err = w.Write(tmp[:8])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
	case "pad":
		sizeExpr, err := g.translateEncodeExpr(parsedTag.bufLenExpr, fields, map[string]bool{})
		if err != nil {
			return err
		}
		if sizeExpr == "" {
			sizeExpr = "1"
		}
		fmt.Fprintf(buf, "\t{\n\t\tm, err = w.Write(make([]byte, %s))\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n", sizeExpr)
	case "string", "bstring", "wstring", "dwstring", "zstring", "z16string":
		encodingOpt := parsedTag.options["encoding"]
		fmt.Fprintf(buf, "\t{\n\t\tstrBytes := []byte(%s)\n", accessor)
		if encodingOpt != "" {
			fmt.Fprintf(buf, "\t\tif ms != nil {\n\t\t\tstrBytes, err = ms.EncodeText(strBytes, %q)\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t}\n", encodingOpt)
		}
		// Write prefix for prefixed strings
		switch binType {
		case "bstring":
			buf.WriteString("\t\ttmp[0] = byte(len(strBytes))\n")
			buf.WriteString("\t\tm, err = w.Write(tmp[:1])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		case "wstring":
			buf.WriteString("\t\torder.PutUint16(tmp[:2], uint16(len(strBytes)))\n")
			buf.WriteString("\t\tm, err = w.Write(tmp[:2])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		case "dwstring":
			buf.WriteString("\t\torder.PutUint32(tmp[:4], uint32(len(strBytes)))\n")
			buf.WriteString("\t\tm, err = w.Write(tmp[:4])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		}
		// Pad or truncate string to buffer size
		if parsedTag.bufLenExpr != "" {
			bufSize, err := g.translateEncodeExpr(parsedTag.bufLenExpr, fields, map[string]bool{})
			if err != nil {
				return err
			}
			fmt.Fprintf(buf, "\t\tbufLen := int(%s)\n", bufSize)
			buf.WriteString("\t\twriteBytes := make([]byte, bufLen)\n")
			buf.WriteString("\t\tcopy(writeBytes, strBytes)\n")
			buf.WriteString("\t\tm, err = w.Write(writeBytes)\n")
		} else {
			buf.WriteString("\t\tm, err = w.Write(strBytes)\n")
		}
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		// Write null termination
		if binType == "zstring" {
			buf.WriteString("\t\ttmp[0] = 0\n\t\tm, err = w.Write(tmp[:1])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		} else if binType == "z16string" {
			buf.WriteString("\t\torder.PutUint16(tmp[:2], 0)\n\t\tm, err = w.Write(tmp[:2])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		}
		buf.WriteString("\t}\n")
	default:
		// Nested struct. When the nested type is itself generated, call its method
		// directly (passing ms through, so nested codecs/encodings/valueofs work) —
		// avoiding a per-value Marshaler allocation and the reflection interpreter.
		// Otherwise fall back to the runtime for that (foreign) type.
		if g.isGeneratedType(goType) {
			fmt.Fprintf(buf, "\t{\n\t\tm, err = (%s).WriteBinaryWithMarshaler(ms, w, order)\n", accessor)
		} else {
			fmt.Fprintf(buf, "\t{\n\t\tm, err = binarystruct.NewMarshalerOrder(order).Write(w, &%s)\n", accessor)
		}
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
	}

	if isPtr {
		buf.WriteString("\t}\n")
	}
	return nil
}

func (g *Generator) generateFieldRead(buf *bytes.Buffer, target, goType, binType string, parsedTag parsedFieldTag, typeName, fieldName, offExpr string) {
	isPtr := strings.HasPrefix(goType, "*")
	accessor := target
	if isPtr {
		accessor = "val"
		fmt.Fprintf(buf, "\t{\n\t\tvar val %s\n", strings.TrimPrefix(goType, "*"))
	}

	if val, ok := parsedTag.options["codec"]; ok && val != "" {
		fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaller context required for custom codec %s\")\n\t}\n", val)
		fmt.Fprintf(buf, "\t{\n\t\tser := ms.GetCodec(%q)\n", val)
		buf.WriteString("\t\tif ser == nil {\n\t\t\treturn n, fmt.Errorf(\"unknown codec: %s\", " + strconv.Quote(val) + ")\n\t\t}\n")
		fmt.Fprintf(buf, "\t\tvalDec, m, err := ser.Decode(r, nil, -1, order)\n")
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
		fmt.Fprintf(buf, "\t\t%s = valDec.(%s)\n\t}\n", accessor, strings.TrimPrefix(goType, "*"))
	} else {
		switch binType {
		case "int8", "uint8", "byte":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:1])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(tmp[0])\n", accessor, strings.TrimPrefix(goType, "*"))
		case "int16", "uint16", "word":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:2])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(order.Uint16(tmp[:2]))\n", accessor, strings.TrimPrefix(goType, "*"))
		case "int32", "uint32", "dword":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:4])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(order.Uint32(tmp[:4]))\n", accessor, strings.TrimPrefix(goType, "*"))
		case "int64", "uint64", "qword":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:8])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(order.Uint64(tmp[:8]))\n", accessor, strings.TrimPrefix(goType, "*"))
		case "float32":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:4])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(math.Float32frombits(order.Uint32(tmp[:4])))\n", accessor, strings.TrimPrefix(goType, "*"))
		case "float64":
			buf.WriteString("\tm, err = io.ReadFull(r, tmp[:8])\n\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			fmt.Fprintf(buf, "\t%s = %s(math.Float64frombits(order.Uint64(tmp[:8])))\n", accessor, strings.TrimPrefix(goType, "*"))
		case "pad":
			sizeExpr := translateExpression(parsedTag.bufLenExpr)
			if sizeExpr == "" {
				sizeExpr = "1"
			}
			fmt.Fprintf(buf, "\t{\n\t\tpadSize := int(%s)\n", sizeExpr)
			buf.WriteString("\t\tm, err = io.ReadFull(r, make([]byte, padSize))\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		case "string", "bstring", "wstring", "dwstring", "zstring", "z16string":
			encodingOpt := parsedTag.options["encoding"]
			buf.WriteString("\t{\n\t\tvar strBytes []byte\n")
			switch binType {
			case "bstring":
				buf.WriteString("\t\tm, err = io.ReadFull(r, tmp[:1])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
				buf.WriteString("\t\tstrLen := int(tmp[0])\n")
				buf.WriteString("\t\tstrBytes = make([]byte, strLen)\n")
				buf.WriteString("\t\tm, err = io.ReadFull(r, strBytes)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
			case "wstring":
				buf.WriteString("\t\tm, err = io.ReadFull(r, tmp[:2])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
				buf.WriteString("\t\tstrLen := int(order.Uint16(tmp[:2]))\n")
				buf.WriteString("\t\tstrBytes = make([]byte, strLen)\n")
				buf.WriteString("\t\tm, err = io.ReadFull(r, strBytes)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
			case "dwstring":
				buf.WriteString("\t\tm, err = io.ReadFull(r, tmp[:4])\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
				buf.WriteString("\t\tstrLen := int(order.Uint32(tmp[:4]))\n")
				buf.WriteString("\t\tstrBytes = make([]byte, strLen)\n")
				buf.WriteString("\t\tm, err = io.ReadFull(r, strBytes)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
			case "zstring":
				buf.WriteString("\t\tfor {\n\t\t\tm, err = io.ReadFull(r, tmp[:1])\n\t\t\tn += m\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t\tif tmp[0] == 0 {\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tstrBytes = append(strBytes, tmp[0])\n\t\t}\n")
			case "z16string":
				buf.WriteString("\t\tfor {\n\t\t\tm, err = io.ReadFull(r, tmp[:2])\n\t\t\tn += m\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t\tval := order.Uint16(tmp[:2])\n\t\t\tif val == 0 {\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tstrBytes = append(strBytes, byte(val), byte(val>>8)) // UTF-16 bytes\n\t\t}\n")
			default:
				if parsedTag.bufLenExpr != "" {
					fmt.Fprintf(buf, "\t\tstrLen := int(%s)\n", translateExpression(parsedTag.bufLenExpr))
					buf.WriteString("\t\tstrBytes = make([]byte, strLen)\n")
					buf.WriteString("\t\tm, err = io.ReadFull(r, strBytes)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
				} else {
					buf.WriteString("\t\t// Read all remaining\n\t\tstrBytes, err = io.ReadAll(r)\n\t\tn += len(strBytes)\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
				}
			}

			if encodingOpt != "" {
				fmt.Fprintf(buf, "\t\tif ms != nil {\n\t\t\tstrBytes, err = ms.DecodeText(strBytes, %q)\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t}\n", encodingOpt)
			}
			// Remove trailing zeros
			buf.WriteString("\t\tstrlen := len(strBytes)\n")
			buf.WriteString("\t\tfor ; strlen > 0 && strBytes[strlen-1] == 0; strlen-- {}\n")
			fmt.Fprintf(buf, "\t\t%s = string(strBytes[:strlen])\n", accessor)
			buf.WriteString("\t}\n")
		default:
			// Nested struct: direct generated-method call when the nested type is
			// itself generated (ms passed through); runtime fallback otherwise.
			if g.isGeneratedType(goType) {
				fmt.Fprintf(buf, "\t{\n\t\tm, err = (%s).ReadBinaryWithMarshaler(ms, r, order)\n", accessor)
			} else {
				fmt.Fprintf(buf, "\t{\n\t\tm, err = binarystruct.NewMarshalerOrder(order).Read(r, &%s)\n", accessor)
			}
			buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		}
	}

	// Apply range check if specified (unless -no-validate strips decode validation)
	if rangeOpt, ok := parsedTag.options["range"]; ok && !g.NoValidate {
		bounds := strings.Split(rangeOpt, "..")
		if len(bounds) == 2 {
			minStr := strings.TrimSpace(bounds[0])
			maxStr := strings.TrimSpace(bounds[1])
			if minStr != "" {
				fmt.Fprintf(buf, "\tif %s < %s {\n", accessor, minStr)
				buf.WriteString(cgValidationErr(offExpr, fieldName, fmt.Sprintf("fmt.Errorf(\"value %%v is out of range [%s..%s]: %%w\", %s, binarystruct.ErrValidationError)", minStr, maxStr, accessor)))
				buf.WriteString("\t}\n")
			}
			if maxStr != "" {
				fmt.Fprintf(buf, "\tif %s > %s {\n", accessor, maxStr)
				buf.WriteString(cgValidationErr(offExpr, fieldName, fmt.Sprintf("fmt.Errorf(\"value %%v is out of range [%s..%s]: %%w\", %s, binarystruct.ErrValidationError)", minStr, maxStr, accessor)))
				buf.WriteString("\t}\n")
			}
		}
	}

	// Apply regex match check (unless -no-validate strips decode validation)
	if _, ok := parsedTag.options["match"]; ok && !g.NoValidate {
		fmt.Fprintf(buf, "\tif !regex_%s_%s.MatchString(%s) {\n", typeName, fieldName, accessor)
		buf.WriteString(cgValidationErr(offExpr, fieldName, fmt.Sprintf("fmt.Errorf(\"value %%q does not match pattern: %%w\", %s, binarystruct.ErrValidationError)", accessor)))
		buf.WriteString("\t}\n")
	}

	if isPtr {
		fmt.Fprintf(buf, "\t\t%s = &val\n\t}\n", target)
	}
}

// multidimLeafTag derives the per-element tag for a multidimensional array leaf:
// the array dimensions are stripped, and per-element validation (const/range/
// match) is dropped (the runtime validates the field as a whole, not per element).
func multidimLeafTag(parsedTag parsedFieldTag) parsedFieldTag {
	leafTag := parsedTag
	leafTag.isArray = false
	leafTag.numDims = 0
	leafTag.arrayLenExpr = ""
	leafTag.arrayDimExprs = nil
	leafTag.options = map[string]string{}
	for k, v := range parsedTag.options {
		switch k {
		case "const", "range", "match":
			// per-element validation of a multidimensional leaf is out of scope
		default:
			leafTag.options[k] = v
		}
	}
	return leafTag
}

// generateMultidimWrite emits nested loops (one per dimension) that write each
// scalar leaf in row-major order. Each level is bounded by len() so the same code
// serves fixed arrays and slices. Supportability is checked in generateMethods.
func (g *Generator) generateMultidimWrite(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo) error {
	_, leaf, _ := cgArrayLevels(goType, parsedTag.numDims)
	leafTag := multidimLeafTag(parsedTag)
	buf.WriteString("\t{\n")
	accessor := "s." + fieldName
	for k := 0; k < parsedTag.numDims; k++ {
		idx := fmt.Sprintf("i%d", k)
		fmt.Fprintf(buf, "\tfor %s := 0; %s < len(%s); %s++ {\n", idx, idx, accessor, idx)
		accessor += "[" + idx + "]"
	}
	if err := g.generateFieldWrite(buf, accessor, leaf, binType, leafTag, fields); err != nil {
		return err
	}
	for k := 0; k < parsedTag.numDims; k++ {
		buf.WriteString("\t}\n")
	}
	buf.WriteString("\t}\n")
	return nil
}

// generateMultidimRead emits nested loops that read each scalar leaf in row-major
// order. Slice levels are allocated with make() to their declared dimension;
// fixed-array levels are iterated by len(). Supportability is checked upstream.
func (g *Generator) generateMultidimRead(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, typeName, offExpr string) {
	isSlice, leaf, _ := cgArrayLevels(goType, parsedTag.numDims)
	leafTag := multidimLeafTag(parsedTag)
	buf.WriteString("\t{\n")
	accessor := "s." + fieldName
	for k := 0; k < parsedTag.numDims; k++ {
		idx := fmt.Sprintf("i%d", k)
		if isSlice[k] {
			fmt.Fprintf(buf, "\t%s = make(%s, int(%s))\n", accessor, cgPeelArrayLevels(goType, k), translateExpression(parsedTag.arrayDimExprs[k]))
		}
		fmt.Fprintf(buf, "\tfor %s := 0; %s < len(%s); %s++ {\n", idx, idx, accessor, idx)
		accessor += "[" + idx + "]"
	}
	g.generateFieldRead(buf, accessor, leaf, binType, leafTag, typeName, fieldName, offExpr)
	for k := 0; k < parsedTag.numDims; k++ {
		buf.WriteString("\t}\n")
	}
	buf.WriteString("\t}\n")
}

// cgArrayCanBulk reports whether an array/slice field's elements can use the bulk
// scalar-buffer path: a fixed-width scalar element (byte/uint8 excluded — they have
// their own direct path), with no per-element validation/codec and a non-pointer
// element type. It returns the element's wire width.
func cgArrayCanBulk(goType, binType string, parsedTag parsedFieldTag) (width int, ok bool) {
	switch binType {
	case "int8":
		width = 1
	case "int16", "uint16", "word":
		width = 2
	case "int32", "uint32", "dword", "float32":
		width = 4
	case "int64", "uint64", "qword", "float64":
		width = 8
	default:
		return 0, false
	}
	elem := goType[strings.IndexByte(goType, ']')+1:] // element type of [] or [N]
	if strings.HasPrefix(elem, "*") {
		return 0, false
	}
	for _, opt := range []string{"range", "match", "const", "codec"} {
		if _, has := parsedTag.options[opt]; has {
			return 0, false
		}
	}
	return width, true
}

// cgGoScalarWidth returns the in-memory size of a fixed-width Go scalar type
// name. Platform-dependent types (int/uint/uintptr) return ok=false, so they
// never qualify for the raw-memory bulk path.
func cgGoScalarWidth(elem string) (int, bool) {
	switch elem {
	case "int8", "uint8", "byte":
		return 1, true
	case "int16", "uint16":
		return 2, true
	case "int32", "uint32", "float32":
		return 4, true
	case "int64", "uint64", "float64":
		return 8, true
	}
	return 0, false
}

// cgArrayCanBulkUnsafe reports whether a scalar array/slice can use the unsafe
// raw-memory bulk path: a single Write/ReadFull over the element backing store
// plus one in-place binarystruct.SwapBytes when the order differs from the host.
// It requires bulk eligibility AND that the Go element's in-memory width equals
// the wire width (so the []byte view has the right stride) — e.g. []int tagged
// int32 is excluded because int is 8 bytes on amd64. width==1 is excluded: it
// has its own direct byte path and needs no swap.
func (g *Generator) cgArrayCanBulkUnsafe(goType, binType string, parsedTag parsedFieldTag) (width int, ok bool) {
	if !g.UnsafeBulk {
		return 0, false
	}
	w, bulkOK := cgArrayCanBulk(goType, binType, parsedTag)
	if !bulkOK || w == 1 {
		return 0, false
	}
	elem := goType[strings.IndexByte(goType, ']')+1:]
	if gw, known := cgGoScalarWidth(elem); !known || gw != w {
		return 0, false
	}
	return w, true
}

// generateScalarSliceBulkWrite emits a single buffer fill + one w.Write for a
// fixed-width scalar array/slice, replacing N per-element order.PutUintN + w.Write.
func (g *Generator) generateScalarSliceBulkWrite(buf *bytes.Buffer, fieldName, binType, sizeExpr string, width int) {
	fmt.Fprintf(buf, "\t{\n\t\twriteLen := int(%s)\n", sizeExpr)
	fmt.Fprintf(buf, "\t\tsbuf := make([]byte, writeLen*%d)\n", width)
	buf.WriteString("\t\tfor i := 0; i < writeLen; i++ {\n")
	switch {
	case width == 1:
		fmt.Fprintf(buf, "\t\t\tsbuf[i] = byte(s.%s[i])\n", fieldName)
	case binType == "float32":
		fmt.Fprintf(buf, "\t\t\torder.PutUint32(sbuf[i*4:], math.Float32bits(float32(s.%s[i])))\n", fieldName)
	case binType == "float64":
		fmt.Fprintf(buf, "\t\t\torder.PutUint64(sbuf[i*8:], math.Float64bits(float64(s.%s[i])))\n", fieldName)
	case width == 2:
		fmt.Fprintf(buf, "\t\t\torder.PutUint16(sbuf[i*2:], uint16(s.%s[i]))\n", fieldName)
	case width == 4:
		fmt.Fprintf(buf, "\t\t\torder.PutUint32(sbuf[i*4:], uint32(s.%s[i]))\n", fieldName)
	case width == 8:
		fmt.Fprintf(buf, "\t\t\torder.PutUint64(sbuf[i*8:], uint64(s.%s[i]))\n", fieldName)
	}
	buf.WriteString("\t\t}\n")
	buf.WriteString("\t\tm, err = w.Write(sbuf)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
}

// generateScalarSliceBulkRead emits a single io.ReadFull + buffer decode for a
// fixed-width scalar array/slice, replacing N per-element io.ReadFull.
func (g *Generator) generateScalarSliceBulkRead(buf *bytes.Buffer, fieldName, goType, binType string, width int, isFixed bool, sizeExpr string) {
	elem := goType[strings.IndexByte(goType, ']')+1:]
	buf.WriteString("\t{\n")
	lenExpr := fmt.Sprintf("len(s.%s)", fieldName)
	if !isFixed {
		fmt.Fprintf(buf, "\t\treadLen := int(%s)\n", sizeExpr)
		fmt.Fprintf(buf, "\t\ts.%s = make(%s, readLen)\n", fieldName, goType)
		lenExpr = "readLen"
	}
	fmt.Fprintf(buf, "\t\tsbuf := make([]byte, %s*%d)\n", lenExpr, width)
	buf.WriteString("\t\tm, err = io.ReadFull(r, sbuf)\n\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
	var get string
	switch {
	case width == 1:
		get = "sbuf[i]"
	case binType == "float32":
		get = "math.Float32frombits(order.Uint32(sbuf[i*4:]))"
	case binType == "float64":
		get = "math.Float64frombits(order.Uint64(sbuf[i*8:]))"
	case width == 2:
		get = "order.Uint16(sbuf[i*2:])"
	case width == 4:
		get = "order.Uint32(sbuf[i*4:])"
	case width == 8:
		get = "order.Uint64(sbuf[i*8:])"
	}
	fmt.Fprintf(buf, "\t\tfor i := 0; i < %s; i++ {\n", lenExpr)
	fmt.Fprintf(buf, "\t\t\ts.%s[i] = %s(%s)\n", fieldName, elem, get)
	buf.WriteString("\t\t}\n\t}\n")
}

// generateScalarSliceBulkWriteUnsafe emits the raw-memory write: when order ==
// host, write the element backing store directly (zero copy); otherwise copy it
// and byte-swap once via binarystruct.SwapBytes (SIMD-accelerated under
// experiment_simd). Replaces the per-element order.PutUintN loop. The caller
// guarantees the Go element width equals `width` (cgArrayCanBulkUnsafe).
func (g *Generator) generateScalarSliceBulkWriteUnsafe(buf *bytes.Buffer, fieldName, sizeExpr string, width int) {
	fmt.Fprintf(buf, "\t{\n\t\twriteLen := int(%s)\n", sizeExpr)
	buf.WriteString("\t\tif writeLen > 0 {\n")
	fmt.Fprintf(buf, "\t\t\tsrc := unsafe.Slice((*byte)(unsafe.Pointer(&s.%s[0])), writeLen*%d)\n", fieldName, width)
	buf.WriteString("\t\t\tif order == binarystruct.HostEndian() {\n")
	buf.WriteString("\t\t\t\tm, err = w.Write(src)\n")
	buf.WriteString("\t\t\t} else {\n")
	fmt.Fprintf(buf, "\t\t\t\tsbuf := make([]byte, writeLen*%d)\n", width)
	buf.WriteString("\t\t\t\tcopy(sbuf, src)\n")
	fmt.Fprintf(buf, "\t\t\t\tbinarystruct.SwapBytes(sbuf, %d)\n", width)
	buf.WriteString("\t\t\t\tm, err = w.Write(sbuf)\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tn += m\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n")
	buf.WriteString("\t\t}\n\t}\n")
}

// generateScalarSliceBulkReadUnsafe emits the raw-memory read: ReadFull straight
// into the element backing store, then one in-place binarystruct.SwapBytes when
// order != host. Replaces the io.ReadFull + per-element order.UintN decode loop.
func (g *Generator) generateScalarSliceBulkReadUnsafe(buf *bytes.Buffer, fieldName, goType string, width int, isFixed bool, sizeExpr string) {
	buf.WriteString("\t{\n")
	lenExpr := fmt.Sprintf("len(s.%s)", fieldName)
	if !isFixed {
		fmt.Fprintf(buf, "\t\treadLen := int(%s)\n", sizeExpr)
		fmt.Fprintf(buf, "\t\ts.%s = make(%s, readLen)\n", fieldName, goType)
		lenExpr = "readLen"
	}
	fmt.Fprintf(buf, "\t\tif %s > 0 {\n", lenExpr)
	fmt.Fprintf(buf, "\t\t\tdst := unsafe.Slice((*byte)(unsafe.Pointer(&s.%s[0])), %s*%d)\n", fieldName, lenExpr, width)
	buf.WriteString("\t\t\tm, err = io.ReadFull(r, dst)\n\t\t\tn += m\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n")
	fmt.Fprintf(buf, "\t\t\tif order != binarystruct.HostEndian() {\n\t\t\t\tbinarystruct.SwapBytes(dst, %d)\n\t\t\t}\n", width)
	buf.WriteString("\t\t}\n\t}\n")
}

func (g *Generator) generateArrayWrite(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo) error {
	if parsedTag.numDims > 1 {
		return g.generateMultidimWrite(buf, fieldName, goType, binType, parsedTag, fields)
	}
	// Encode path: resolve valueof-referenced length fields to their computed
	// values so e.g. [NameLen]byte writes len(s.Name) bytes, not stale s.NameLen.
	sizeExpr, err := g.translateEncodeExpr(parsedTag.arrayLenExpr, fields, map[string]bool{})
	if err != nil {
		return err
	}
	if sizeExpr == "" {
		sizeExpr = fmt.Sprintf("len(s.%s)", fieldName)
	}

	if goType == "string" {
		if binType == "byte" || binType == "uint8" {
			fmt.Fprintf(buf, "\t{\n\t\twriteLen := int(%s)\n", sizeExpr)
			buf.WriteString("\t\tstrBytes := make([]byte, writeLen)\n")
			fmt.Fprintf(buf, "\t\tcopy(strBytes, s.%s)\n", fieldName)
			buf.WriteString("\t\tm, err = w.Write(strBytes)\n")
			buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
			return nil
		}
	}

	// Bulk write optimization for byte slices
	if binType == "byte" || binType == "uint8" {
		fmt.Fprintf(buf, "\t{\n\t\twriteLen := int(%s)\n", sizeExpr)
		fmt.Fprintf(buf, "\t\tm, err = w.Write(s.%s[:writeLen])\n", fieldName)
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		return nil
	}

	// Bulk write for fixed-width multibyte scalar slices: one buffer + one Write
	// instead of a per-element order.PutUintN + w.Write. When the Go element width
	// matches the wire width, take the raw-memory path (one swap via SwapBytes,
	// SIMD-accelerated under experiment_simd); otherwise the per-element buffer fill.
	if width, ok := g.cgArrayCanBulkUnsafe(goType, binType, parsedTag); ok {
		g.generateScalarSliceBulkWriteUnsafe(buf, fieldName, sizeExpr, width)
		return nil
	}
	if width, ok := cgArrayCanBulk(goType, binType, parsedTag); ok {
		g.generateScalarSliceBulkWrite(buf, fieldName, binType, sizeExpr, width)
		return nil
	}

	fmt.Fprintf(buf, "\t{\n\t\tlimit := int(%s)\n", sizeExpr)
	buf.WriteString("\t\tfor i := 0; i < limit; i++ {\n")
	if err := g.generateFieldWrite(buf, fmt.Sprintf("s.%s[i]", fieldName), strings.TrimPrefix(goType, "[]"), binType, parsedTag, fields); err != nil {
		return err
	}
	buf.WriteString("\t\t}\n\t}\n")
	return nil
}

func (g *Generator) generateArrayRead(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, typeName, offExpr string) {
	if parsedTag.numDims > 1 {
		g.generateMultidimRead(buf, fieldName, goType, binType, parsedTag, typeName, offExpr)
		return
	}
	sizeExpr := translateExpression(parsedTag.arrayLenExpr)
	if sizeExpr == "" {
		buf.WriteString("\treturn n, errors.New(\"unknown array size expression\")\n")
		return
	}

	if goType == "string" {
		if binType == "byte" || binType == "uint8" {
			fmt.Fprintf(buf, "\t{\n\t\treadLen := int(%s)\n", sizeExpr)
			buf.WriteString("\t\tstrBytes := make([]byte, readLen)\n")
			buf.WriteString("\t\tm, err = io.ReadFull(r, strBytes)\n")
			buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n")
			buf.WriteString("\t\tstrlen := len(strBytes)\n")
			buf.WriteString("\t\tfor ; strlen > 0 && strBytes[strlen-1] == 0; strlen-- {}\n")
			fmt.Fprintf(buf, "\t\ts.%s = string(strBytes[:strlen])\n", fieldName)
			buf.WriteString("\t}\n")
			return
		}
	}

	// Fixed [N]T array: read in place; make() is only valid for slices.
	if isFixedArrayType(goType) {
		if binType == "byte" || binType == "uint8" {
			fmt.Fprintf(buf, "\tm, err = io.ReadFull(r, s.%s[:])\n", fieldName)
			buf.WriteString("\tn += m\n\tif err != nil {\n\t\treturn n, err\n\t}\n")
			return
		}
		if width, ok := g.cgArrayCanBulkUnsafe(goType, binType, parsedTag); ok {
			g.generateScalarSliceBulkReadUnsafe(buf, fieldName, goType, width, true, sizeExpr)
			return
		}
		if width, ok := cgArrayCanBulk(goType, binType, parsedTag); ok {
			g.generateScalarSliceBulkRead(buf, fieldName, goType, binType, width, true, sizeExpr)
			return
		}
		elemType := goType[strings.IndexByte(goType, ']')+1:]
		fmt.Fprintf(buf, "\tfor i := 0; i < len(s.%s); i++ {\n", fieldName)
		g.generateFieldRead(buf, fmt.Sprintf("s.%s[i]", fieldName), elemType, binType, parsedTag, typeName, fmt.Sprintf("%s[i]", fieldName), offExpr)
		buf.WriteString("\t}\n")
		return
	}

	// Bulk read for fixed-width multibyte scalar slices: one io.ReadFull + buffer
	// decode instead of a per-element io.ReadFull. Prefer the raw-memory path
	// (ReadFull into the backing store + one SwapBytes) when widths match.
	if width, ok := g.cgArrayCanBulkUnsafe(goType, binType, parsedTag); ok {
		g.generateScalarSliceBulkReadUnsafe(buf, fieldName, goType, width, false, sizeExpr)
		return
	}
	if width, ok := cgArrayCanBulk(goType, binType, parsedTag); ok {
		g.generateScalarSliceBulkRead(buf, fieldName, goType, binType, width, false, sizeExpr)
		return
	}

	// Slice initialization
	fmt.Fprintf(buf, "\t{\n\t\treadLen := int(%s)\n", sizeExpr)
	fmt.Fprintf(buf, "\t\ts.%s = make(%s, readLen)\n", fieldName, goType)

	// Bulk read optimization for byte slices
	if binType == "byte" || binType == "uint8" {
		fmt.Fprintf(buf, "\t\tm, err = io.ReadFull(r, s.%s)\n", fieldName)
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		return
	}

	buf.WriteString("\t\tfor i := 0; i < readLen; i++ {\n")
	g.generateFieldRead(buf, fmt.Sprintf("s.%s[i]", fieldName), strings.TrimPrefix(goType, "[]"), binType, parsedTag, typeName, fmt.Sprintf("%s[i]", fieldName), offExpr)
	buf.WriteString("\t\t}\n\t}\n")
}

func getEffectiveBinaryType(binType, goType string) string {
	if binType != "" && binType != "any" {
		return binType
	}
	goType = strings.TrimPrefix(goType, "*")
	goType = strings.TrimPrefix(goType, "[]")
	return goType
}

func (g *Generator) GenerateJSON(outPath string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, g.Dir, func(fi os.FileInfo) bool {
		if fi.Name() == filepath.Base(outPath) {
			return false
		}
		if strings.HasSuffix(fi.Name(), "_test.go") {
			return g.IncludeTests
		}
		return true
	}, 0)
	if err != nil {
		return fmt.Errorf("failed to parse directory: %w", err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no Go packages found in %s", g.Dir)
	}

	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}

	structs := make(map[string]*ast.StructType)
	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			structs[ts.Name.Name] = st
			return true
		})
	}

	type CodegenField struct {
		Name         string            `json:"name"`
		GoType       string            `json:"go_type"`
		BinaryType   string            `json:"binary_type"`
		IsArray      bool              `json:"is_array"`
		ArrayLenExpr string            `json:"array_len_expr,omitempty"`
		BufLenExpr   string            `json:"buf_len_expr,omitempty"`
		Options      map[string]string `json:"options,omitempty"`
	}

	type CodegenStruct struct {
		Name   string         `json:"name"`
		Fields []CodegenField `json:"fields"`
	}

	var result []CodegenStruct

	for _, typeName := range g.Types {
		st, ok := structs[typeName]
		if !ok {
			return fmt.Errorf("type %s not found in package", typeName)
		}
		var fields []CodegenField
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 || field.Names[0].Name == "_" {
				continue
			}
			fieldName := field.Names[0].Name
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)
			if !parsedTag.hasTag {
				continue
			}
			binType := getEffectiveBinaryType(parsedTag.binaryType, goType)

			fields = append(fields, CodegenField{
				Name:         fieldName,
				GoType:       goType,
				BinaryType:   binType,
				IsArray:      parsedTag.isArray,
				ArrayLenExpr: parsedTag.arrayLenExpr,
				BufLenExpr:   parsedTag.bufLenExpr,
				Options:      parsedTag.options,
			})
		}
		result = append(result, CodegenStruct{
			Name:   typeName,
			Fields: fields,
		})
	}

	js, err := json.MarshalIndent(result, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if outPath == "" {
		_, err = os.Stdout.Write(js)
		return err
	}

	return ioutil.WriteFile(outPath, js, 0644)
}
