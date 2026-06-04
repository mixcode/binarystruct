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
	"strconv"
	"strings"
)

type Generator struct {
	Dir          string
	Types        []string
	IncludeTests bool

	// structs holds every struct type parsed from the target package, keyed by
	// name. Populated by Generate; used to recognize nested-struct fields when
	// translating bytelen() (case 5).
	structs map[string]*ast.StructType
}

type parsedFieldTag struct {
	hasTag       bool
	binaryType   string
	isArray      bool
	arrayLenExpr string
	bufLenExpr   string
	options      map[string]string
}

var mTag = regexp.MustCompile(`^\s*(\[([^\]]*)\])?([^\s\(\)]*)(\(([^\)]+)\))?`)

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

	tags := strings.Split(tagStr, ",")
	if len(tags) == 0 || tags[0] == "" {
		return res
	}

	match := mTag.FindStringSubmatch(tags[0])
	if len(match) >= 6 {
		res.isArray = match[1] != ""
		res.arrayLenExpr = match[2]
		res.binaryType = match[3]
		res.bufLenExpr = match[5]
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
)

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
func (g *Generator) translateValueof(expr string, fields map[string]cgFieldInfo) (pre string, out string, err error) {
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
			repl, p, e := g.bytelenExpr(arg, fi, fields, measured)
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
func (g *Generator) bytelenExpr(arg string, fi cgFieldInfo, fields map[string]cgFieldInfo, measured map[string]bool) (expr, pre string, err error) {
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
			bufSize, e := g.translateEncodeExpr(fi.bufLenExpr, fields)
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
			pre = fmt.Sprintf("\tvar %s int\n\tif s.%s != nil {\n\t\t%s, err = binarystruct.Write(io.Discard, order, s.%s)\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n", tmp, arg, tmp, arg)
			return tmp, pre, nil
		}
		if fi.isArray {
			// A tag-counted array of structs ([N]Elem). Mirror the encode loop:
			// same element count and the same per-element binarystruct.Write.
			sizeExpr, e := g.translateEncodeExpr(fi.arrayLenExpr, fields)
			if e != nil {
				return "", "", e
			}
			if sizeExpr == "" {
				sizeExpr = fmt.Sprintf("len(s.%s)", arg)
			}
			pre = fmt.Sprintf("\tvar %s int\n\t{\n\t\tlimit := int(%s)\n\t\tfor i := 0; i < limit; i++ {\n\t\t\tvar mm int\n\t\t\tmm, err = binarystruct.Write(io.Discard, order, &s.%s[i])\n\t\t\tif err != nil {\n\t\t\t\treturn n, err\n\t\t\t}\n\t\t\t%s += mm\n\t\t}\n\t}\n", tmp, sizeExpr, arg, tmp)
			return tmp, pre, nil
		}
		// A single struct value, or a Go-native slice/array of structs written as
		// a whole value: the write path encodes it with one binarystruct.Write.
		pre = fmt.Sprintf("\tvar %s int\n\t%s, err = binarystruct.Write(io.Discard, order, &s.%s)\n\tif err != nil {\n\t\treturn n, err\n\t}\n", tmp, tmp, arg)
		return tmp, pre, nil
	}

	// case 2: fixed-width scalar or scalar array/slice -> width * count.
	if w, ok := scalarWidth(fi.binType); ok {
		if strings.HasPrefix(fi.goType, "*") {
			// A pointer scalar may be nil (0 bytes); a constant width would be wrong.
			return "", "", fmt.Errorf("codegen does not support bytelen(%s) of a pointer scalar field; use the runtime interpreter for this struct", arg)
		}
		if fi.isArray {
			sizeExpr, e := g.translateEncodeExpr(fi.arrayLenExpr, fields)
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
func (g *Generator) translateEncodeExpr(expr string, fields map[string]cgFieldInfo) (string, error) {
	if expr == "" {
		return "", nil
	}
	prefixed := translateExpression(expr)
	var ferr error
	out := cgIdentRe.ReplaceAllStringFunc(prefixed, func(m string) string {
		name := cgIdentRe.FindStringSubmatch(m)[1]
		fi, ok := fields[name]
		if ok && fi.hasValueof {
			pre, sub, err := g.translateValueof(fi.valueofExpr, fields)
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

	for _, typeName := range g.Types {
		st, ok := structs[typeName]
		if !ok {
			return fmt.Errorf("type %s not found in package %s", typeName, pkgName)
		}
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)
			binType := getEffectiveBinaryType(parsedTag.binaryType, goType)

			if binType == "float32" || binType == "float64" {
				needMath = true
			}
			if _, ok := parsedTag.options["match"]; ok {
				needRegexp = true
				needFmt = true
			}
			if _, ok := parsedTag.options["range"]; ok {
				needFmt = true
			}
			if cexpr, ok := parsedTag.options["const"]; ok && cexpr != "" {
				needFmt = true
			}
			if val, ok := parsedTag.options["serializer"]; ok && val != "" {
				needErrors = true
				needFmt = true
			}
			if parsedTag.isArray && parsedTag.arrayLenExpr == "" {
				needErrors = true
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
	buf.WriteString("\t\"github.com/mixcode/binarystruct\"\n")
	buf.WriteString(")\n\n")

	// Generate regex variables for match checks
	for _, typeName := range g.Types {
		st, ok := structs[typeName]
		if !ok {
			return fmt.Errorf("type %s not found in package %s", typeName, pkgName)
		}
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			fieldName := field.Names[0].Name
			parsedTag := parseFieldTag(field.Tag)
			if pattern, ok := parsedTag.options["match"]; ok {
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

func (g *Generator) generateMethods(buf *bytes.Buffer, typeName string, st *ast.StructType) error {
	// Write standard helper functions
	fmt.Fprintf(buf, "// MarshalBinary implements encoding.BinaryMarshaler.\n")
	fmt.Fprintf(buf, "func (s *%s) MarshalBinary() ([]byte, error) {\n", typeName)
	buf.WriteString("\tvar b bytes.Buffer\n")
	buf.WriteString("\t_, err := s.WriteBinary(&b, binarystruct.BigEndian)\n")
	buf.WriteString("\treturn b.Bytes(), err\n")
	buf.WriteString("}\n\n")

	fmt.Fprintf(buf, "// UnmarshalBinary implements encoding.BinaryUnmarshaler.\n")
	fmt.Fprintf(buf, "func (s *%s) UnmarshalBinary(data []byte) error {\n", typeName)
	buf.WriteString("\tr := bytes.NewReader(data)\n")
	buf.WriteString("\t_, err := s.ReadBinary(r, binarystruct.BigEndian)\n")
	buf.WriteString("\treturn err\n")
	buf.WriteString("}\n\n")

	// 1. WriteBinary (Standard)
	fmt.Fprintf(buf, "// WriteBinary implements binarystruct.BinaryWriter.\n")
	fmt.Fprintf(buf, "func (s *%s) WriteBinary(w io.Writer, order binarystruct.ByteOrder) (int, error) {\n", typeName)
	buf.WriteString("\treturn s.WriteBinaryWithMarshaller(nil, w, order)\n")
	buf.WriteString("}\n\n")

	// 2. WriteBinaryWithMarshaller (Context-aware)
	fmt.Fprintf(buf, "// WriteBinaryWithMarshaller implements binarystruct.MarshallerContextWriter.\n")
	fmt.Fprintf(buf, "func (s *%s) WriteBinaryWithMarshaller(ms *binarystruct.Marshaller, w io.Writer, order binarystruct.ByteOrder) (n int, err error) {\n", typeName)

	// Field info for resolving valueof's bytelen()/count() at generation time.
	fieldInfo := make(map[string]cgFieldInfo)
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		pt := parseFieldTag(field.Tag)
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
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			fieldName := field.Names[0].Name
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)

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
				pre, valExpr, vErr := g.translateValueof(vexpr, fieldInfo)
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
	emitLocalScratch(buf, writeBody.String())
	buf.Write(writeBody.Bytes())
	buf.WriteString("\treturn n, nil\n")
	buf.WriteString("}\n\n")

	// 3. ReadBinary (Standard)
	fmt.Fprintf(buf, "// ReadBinary implements binarystruct.BinaryReader.\n")
	fmt.Fprintf(buf, "func (s *%s) ReadBinary(r io.Reader, order binarystruct.ByteOrder) (int, error) {\n", typeName)
	buf.WriteString("\treturn s.ReadBinaryWithMarshaller(nil, r, order)\n")
	buf.WriteString("}\n\n")

	// 4. ReadBinaryWithMarshaller (Context-aware)
	fmt.Fprintf(buf, "// ReadBinaryWithMarshaller implements binarystruct.MarshallerContextReader.\n")
	fmt.Fprintf(buf, "func (s *%s) ReadBinaryWithMarshaller(ms *binarystruct.Marshaller, r io.Reader, order binarystruct.ByteOrder) (n int, err error) {\n", typeName)

	var readBody bytes.Buffer
	if err := func() error {
		buf := &readBody
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			fieldName := field.Names[0].Name
			goType := getGoTypeName(field.Type)
			parsedTag := parseFieldTag(field.Tag)

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

			if parsedTag.isArray {
				g.generateArrayRead(buf, fieldName, goType, binType, parsedTag, typeName)
			} else {
				g.generateFieldRead(buf, "s."+fieldName, goType, binType, parsedTag, typeName, fieldName)
			}

			// const: validate the field equals its fixed value after reading.
			if cexpr, ok := parsedTag.options["const"]; ok && cexpr != "" {
				if err := g.generateConstValidate(buf, fieldName, goType, binType, cexpr); err != nil {
					return fmt.Errorf("field %s: %w", fieldName, err)
				}
			}
		}
		return nil
	}(); err != nil {
		return err
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

// generateConstValidate emits a post-read check that the field equals its const.
func (g *Generator) generateConstValidate(buf *bytes.Buffer, fieldName, goType, binType, cexpr string) error {
	accessor := "s." + fieldName
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
		fmt.Fprintf(buf, "\t\treturn n, fmt.Errorf(\"field %s: const mismatch: %%w\", binarystruct.ErrValidationError)\n\t}\n", fieldName)
		return nil
	}
	fmt.Fprintf(buf, "\tif %s != (%s) {\n", accessor, cexpr)
	fmt.Fprintf(buf, "\t\treturn n, fmt.Errorf(\"field %s: const mismatch: %%w\", binarystruct.ErrValidationError)\n\t}\n", fieldName)
	return nil
}

func (g *Generator) generateFieldWrite(buf *bytes.Buffer, target, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo) error {
	// Dereference pointer if necessary
	isPtr := strings.HasPrefix(goType, "*")
	accessor := target
	if isPtr {
		accessor = "*" + target
		fmt.Fprintf(buf, "\tif %s != nil {\n", target)
	}

	if val, ok := parsedTag.options["serializer"]; ok && val != "" {
		fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaller context required for custom serializer %s\")\n\t}\n", val)
		fmt.Fprintf(buf, "\t{\n\t\tser := ms.GetSerializer(%q)\n", val)
		buf.WriteString("\t\tif ser == nil {\n\t\t\treturn n, fmt.Errorf(\"unknown serializer: %s\", " + strconv.Quote(val) + ")\n\t\t}\n")
		fmt.Fprintf(buf, "\t\tm, err = ser.Serialize(w, %s, nil, -1, order)\n", accessor)
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
		sizeExpr, err := g.translateEncodeExpr(parsedTag.bufLenExpr, fields)
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
			bufSize, err := g.translateEncodeExpr(parsedTag.bufLenExpr, fields)
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
		// Nested struct or fallback
		fmt.Fprintf(buf, "\t{\n\t\tm, err = binarystruct.Write(w, order, &%s)\n", accessor)
		buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
	}

	if isPtr {
		buf.WriteString("\t}\n")
	}
	return nil
}

func (g *Generator) generateFieldRead(buf *bytes.Buffer, target, goType, binType string, parsedTag parsedFieldTag, typeName, fieldName string) {
	isPtr := strings.HasPrefix(goType, "*")
	accessor := target
	if isPtr {
		accessor = "val"
		fmt.Fprintf(buf, "\t{\n\t\tvar val %s\n", strings.TrimPrefix(goType, "*"))
	}

	if val, ok := parsedTag.options["serializer"]; ok && val != "" {
		fmt.Fprintf(buf, "\tif ms == nil {\n\t\treturn n, errors.New(\"marshaller context required for custom serializer %s\")\n\t}\n", val)
		fmt.Fprintf(buf, "\t{\n\t\tser := ms.GetSerializer(%q)\n", val)
		buf.WriteString("\t\tif ser == nil {\n\t\t\treturn n, fmt.Errorf(\"unknown serializer: %s\", " + strconv.Quote(val) + ")\n\t\t}\n")
		fmt.Fprintf(buf, "\t\tvalDec, m, err := ser.Deserialize(r, nil, -1, order)\n")
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
			// Nested struct
			fmt.Fprintf(buf, "\t{\n\t\tm, err = binarystruct.Read(r, order, &%s)\n", accessor)
			buf.WriteString("\t\tn += m\n\t\tif err != nil {\n\t\t\treturn n, err\n\t\t}\n\t}\n")
		}
	}

	// Apply range check if specified
	if rangeOpt, ok := parsedTag.options["range"]; ok {
		bounds := strings.Split(rangeOpt, "..")
		if len(bounds) == 2 {
			minStr := strings.TrimSpace(bounds[0])
			maxStr := strings.TrimSpace(bounds[1])
			if minStr != "" {
				fmt.Fprintf(buf, "\tif %s < %s {\n", accessor, minStr)
				fmt.Fprintf(buf, "\t\treturn n, fmt.Errorf(\"field %s: value %%v is out of range [%s..%s]: %%w\", %s, binarystruct.ErrValidationError)\n\t}\n", fieldName, minStr, maxStr, accessor)
			}
			if maxStr != "" {
				fmt.Fprintf(buf, "\tif %s > %s {\n", accessor, maxStr)
				fmt.Fprintf(buf, "\t\treturn n, fmt.Errorf(\"field %s: value %%v is out of range [%s..%s]: %%w\", %s, binarystruct.ErrValidationError)\n\t}\n", fieldName, minStr, maxStr, accessor)
			}
		}
	}

	// Apply regex match check
	if _, ok := parsedTag.options["match"]; ok {
		fmt.Fprintf(buf, "\tif !regex_%s_%s.MatchString(%s) {\n", typeName, fieldName, accessor)
		fmt.Fprintf(buf, "\t\treturn n, fmt.Errorf(\"field %s: value %%q does not match pattern: %%w\", %s, binarystruct.ErrValidationError)\n\t}\n", fieldName, accessor)
	}

	if isPtr {
		fmt.Fprintf(buf, "\t\t%s = &val\n\t}\n", target)
	}
}

func (g *Generator) generateArrayWrite(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, fields map[string]cgFieldInfo) error {
	// Encode path: resolve valueof-referenced length fields to their computed
	// values so e.g. [NameLen]byte writes len(s.Name) bytes, not stale s.NameLen.
	sizeExpr, err := g.translateEncodeExpr(parsedTag.arrayLenExpr, fields)
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

	fmt.Fprintf(buf, "\t{\n\t\tlimit := int(%s)\n", sizeExpr)
	buf.WriteString("\t\tfor i := 0; i < limit; i++ {\n")
	if err := g.generateFieldWrite(buf, fmt.Sprintf("s.%s[i]", fieldName), strings.TrimPrefix(goType, "[]"), binType, parsedTag, fields); err != nil {
		return err
	}
	buf.WriteString("\t\t}\n\t}\n")
	return nil
}

func (g *Generator) generateArrayRead(buf *bytes.Buffer, fieldName, goType, binType string, parsedTag parsedFieldTag, typeName string) {
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
		elemType := goType[strings.IndexByte(goType, ']')+1:]
		fmt.Fprintf(buf, "\tfor i := 0; i < len(s.%s); i++ {\n", fieldName)
		g.generateFieldRead(buf, fmt.Sprintf("s.%s[i]", fieldName), elemType, binType, parsedTag, typeName, fmt.Sprintf("%s[i]", fieldName))
		buf.WriteString("\t}\n")
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
	g.generateFieldRead(buf, fmt.Sprintf("s.%s[i]", fieldName), strings.TrimPrefix(goType, "[]"), binType, parsedTag, typeName, fmt.Sprintf("%s[i]", fieldName))
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
			if len(field.Names) == 0 {
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
