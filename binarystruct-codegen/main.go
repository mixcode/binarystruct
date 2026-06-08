// Copyright 2026 github.com/mixcode

// binarystruct-codegen generates static MarshalBinary/UnmarshalBinary methods
// for structs annotated with `binary:"..."` tags, producing optimized code that
// avoids runtime reflection.
//
// See https://github.com/mixcode/binarystruct for the full library documentation.
//
// Usage:
//
//	binarystruct-codegen -type TypeName[,TypeName2,...] [flags] [directory]
//
// Flags:
//
//	-type string
//	    Comma-separated list of struct type names to generate methods for (required).
//	-endian string
//	    Byte order baked into the no-arg MarshalBinary/UnmarshalBinary/AppendBinary
//	    methods: "big" or "little" (required when generating Go code; not for -json).
//	-output string
//	    Output file name (default: <first_type>_binary.go).
//
// The directory argument specifies the Go package directory containing the struct
// definitions. If omitted, the current directory is used.
//
// Example:
//
//	# Generate methods for Packet and Header types in the current directory
//	binarystruct-codegen -type Packet,Header
//
//	# Generate to a specific output file
//	binarystruct-codegen -type Packet -output packet_gen.go ./protocol
//
//	# Use with go:generate
//	//go:generate binarystruct-codegen -type Packet,Header
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	typeNames    = flag.String("type", "", "comma-separated list of struct type names to generate methods for (required)")
	outputFile   = flag.String("output", "", "output file name; default <first_type>_binary.go (or <first_type>.json if -json is set)")
	includeTests = flag.Bool("tests", false, "include test files (*_test.go) when parsing the package")
	jsonOutput   = flag.Bool("json", false, "generate JSON representation of the struct layout instead of Go source code")
	endian       = flag.String("endian", "", "fallback byte order `big|little` baked into the no-arg MarshalBinary/UnmarshalBinary/AppendBinary methods; optional when the struct declares its own order via a blank _ struct{} endian= field")
	noValidate   = flag.Bool("no-validate", false, "strip ALL decode-time validation from the generated read methods (const/range/match checks and custom valueof recompute-and-compare); default off (the generated decode validates everything, matching the runtime interpreter). Set for trusted-input / hot-path decoding")
)

// orderLiteral maps the -endian flag to the binarystruct byte-order expression
// the generated code uses. There is no default: the stdlib encoding.Binary*
// interfaces carry no order, so the baked order must be chosen explicitly.
func orderLiteral(endian string) (string, error) {
	switch endian {
	case "big":
		return "binarystruct.BigEndian", nil
	case "little":
		return "binarystruct.LittleEndian", nil
	case "":
		return "", fmt.Errorf("missing required -endian flag (\"big\" or \"little\"): the no-arg MarshalBinary/UnmarshalBinary/AppendBinary methods need an explicit byte order")
	default:
		return "", fmt.Errorf("invalid -endian value %q: must be \"big\" or \"little\"", endian)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `binarystruct-codegen: generate static MarshalBinary/UnmarshalBinary methods.

Usage:
  binarystruct-codegen -type TypeName[,TypeName2,...] [flags] [directory]

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Arguments:
  [directory]    Go package directory containing struct definitions (default: ".")

Generated code implements encoding.BinaryMarshaler and encoding.BinaryUnmarshaler
interfaces, producing optimized static methods that bypass runtime reflection.

If the struct uses runtime-dependent features (text encodings, custom codecs, or
custom valueof evaluators such as valueof=CRC32(...)), context-aware methods
(WriteBinaryWithMarshaler/ReadBinaryWithMarshaler) are also generated, allowing the
Marshaler to pass through encodings, codecs, and registered valueof evaluators.

See https://github.com/mixcode/binarystruct for the full library documentation.
`)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("binarystruct-codegen: ")
	flag.Usage = usage
	flag.Parse()

	if *typeNames == "" {
		flag.Usage()
		os.Exit(1)
	}

	types := strings.Split(*typeNames, ",")
	for i, t := range types {
		types[i] = strings.TrimSpace(t)
	}

	// Determine directory
	dir := "."
	args := flag.Args()
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("failed to resolve directory path: %v", err)
	}

	// Default output file name
	out := *outputFile
	if out == "" {
		if *jsonOutput {
			out = strings.ToLower(types[0]) + ".json"
		} else {
			out = strings.ToLower(types[0]) + "_binary.go"
		}
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(absDir, out)
	}

	g := Generator{
		Dir:          absDir,
		Types:        types,
		IncludeTests: *includeTests,
		NoValidate:   *noValidate,
	}

	if *jsonOutput {
		// JSON layout export bakes no byte order, so -endian is not required here.
		if err := g.GenerateJSON(out); err != nil {
			log.Fatalf("JSON generation failed: %v", err)
		}
		fmt.Printf("Generated binarystruct JSON layout for %s -> %s\n", *typeNames, filepath.Base(out))
	} else {
		// -endian is optional: a struct that declares its own order (a blank
		// `_ struct{}` field tagged endian=) supplies it. The flag only sets the
		// fallback baked into the no-arg methods; the generator errors per-type if
		// neither is present. Validate the flag only when it is given.
		if *endian != "" {
			lit, err := orderLiteral(*endian)
			if err != nil {
				log.Print(err)
				flag.Usage()
				os.Exit(1)
			}
			g.Endian = lit
		}
		if err := g.Generate(out); err != nil {
			log.Fatalf("generation failed: %v", err)
		}
		fmt.Printf("Generated binarystruct methods for %s -> %s\n", *typeNames, filepath.Base(out))
	}
}
