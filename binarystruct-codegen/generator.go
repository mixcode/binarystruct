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

// translateValueof converts a valueof expression into a Go integer expression.
// count(F) and bytelen(F) of byte sequences / raw strings become len(s.F);
// arithmetic and parentheses pass through. Cases that cannot be computed inline
// (bytelen of nested structs or text-encoded strings, or a reference to another
// valueof field) return a generation-time error rather than emitting wrong code.
func (g *Generator) translateValueof(expr string, fields map[string]cgFieldInfo) (string, error) {
	prefixed := translateExpression(expr) // e.g. s.bytelen(s.Name)+2
	var ferr error
	out := cgValueofFuncRe.ReplaceAllStringFunc(prefixed, func(m string) string {
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
			if isByteSequence(fi.goType) || (fi.goType == "string" && fi.encoding == "") {
				return fmt.Sprintf("len(s.%s)", arg)
			}
			ferr = fmt.Errorf("codegen does not support bytelen(%s) of type %q (e.g. nested struct or text-encoded string); use the runtime interpreter for this struct", arg, fi.goType)
			return m
		}
		return m
	})
	if ferr != nil {
		return "", ferr
	}
	// A bare reference to another valueof field cannot be resolved in generated
	// code (it would read the field's pre-encode value, diverging from runtime).
	for _, sm := range cgIdentRe.FindAllStringSubmatch(out, -1) {
		if fi, ok := fields[sm[1]]; ok && fi.hasValueof {
			return "", fmt.Errorf("codegen does not support a valueof expression referencing another valueof field (%q); use the runtime interpreter", sm[1])
		}
	}
	return out, nil
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
			sub, err := g.translateValueof(fi.valueofExpr, fields)
			if err != nil {
				ferr = err
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
	buf.WriteString("\tvar tmp [8]byte\n")
	buf.WriteString("\tvar m int\n")

	// Field info for resolving valueof's bytelen()/count() at generation time.
	fieldInfo := make(map[string]cgFieldInfo)
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		pt := parseFieldTag(field.Tag)
		vexpr, hasV := pt.options["valueof"]
		fieldInfo[field.Names[0].Name] = cgFieldInfo{
			goType:      getGoTypeName(field.Type),
			encoding:    pt.options["encoding"],
			hasValueof:  hasV,
			valueofExpr: vexpr,
		}
	}

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
			valExpr, vErr := g.translateValueof(vexpr, fieldInfo)
			if vErr != nil {
				return fmt.Errorf("field %s: %w", fieldName, vErr)
			}
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
	buf.WriteString("\tvar tmp [8]byte\n")
	buf.WriteString("\tvar m int\n")

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
