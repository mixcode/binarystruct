// Copyright 2026 github.com/mixcode

package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

var (
	typeNames  = flag.String("type", "", "comma-separated list of type names; must be set")
	outputFile = flag.String("output", "", "output file name; default <type>_binary.go")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("binarystruct-codegen: ")
	flag.Parse()

	if *typeNames == "" {
		log.Fatal("-type flag is required")
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
		Dir:   absDir,
		Types: types,
	}

	if err := g.Generate(out); err != nil {
		log.Fatalf("generation failed: %v", err)
	}

	fmt.Printf("Generated binarystruct methods for %s -> %s\n", *typeNames, filepath.Base(out))
}
