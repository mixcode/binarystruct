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
	outputFile   = flag.String("output", "", "output file name; default <first_type>_binary.go")
	includeTests = flag.Bool("tests", false, "include test files (*_test.go) when parsing the package")
)

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

If the struct uses runtime-dependent features (text encodings, custom serializers),
context-aware methods (WriteBinaryWithMarshaller/ReadBinaryWithMarshaller) are also
generated, allowing the Marshaller to pass through encodings and serializers.

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
		out = strings.ToLower(types[0]) + "_binary.go"
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(absDir, out)
	}

	g := Generator{
		Dir:          absDir,
		Types:        types,
		IncludeTests: *includeTests,
	}

	if err := g.Generate(out); err != nil {
		log.Fatalf("generation failed: %v", err)
	}

	fmt.Printf("Generated binarystruct methods for %s -> %s\n", *typeNames, filepath.Base(out))
}
