// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

// TestMultidim_FixedArray covers an explicit [2][3]int16 tag on a Go fixed array.
func TestMultidim_FixedArray(t *testing.T) {
	type S struct {
		_ struct{}    `binary:"endian=big"`
		M [2][3]int16 `binary:"[2][3]int16"`
	}
	in := S{M: [2][3]int16{{1, 2, 3}, {4, 5, 6}}}
	want := []byte{0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6}

	b, err := bst.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("encode = % x, want % x", b, want)
	}
	var out S
	if _, err := bst.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.M != in.M {
		t.Fatalf("round trip mismatch: %v vs %v", out.M, in.M)
	}
}

// TestMultidim_Slices covers nested slices [][]int16 with an explicit [2][3] tag,
// including decode allocation of both levels.
func TestMultidim_Slices(t *testing.T) {
	type S struct {
		_ struct{}  `binary:"endian=little"`
		M [][]int16 `binary:"[2][3]int16"`
	}
	in := S{M: [][]int16{{1, 2, 3}, {4, 5, 6}}}
	want := []byte{1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6, 0}

	b, err := bst.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("encode = % x, want % x", b, want)
	}
	var out S
	if _, err := bst.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.M) != 2 || len(out.M[0]) != 3 || out.M[1][2] != 6 {
		t.Fatalf("round trip shape/values wrong: %v", out.M)
	}
}

// TestMultidim_ThreeD covers the TODO's example shape [2][2][2]int8.
func TestMultidim_ThreeD(t *testing.T) {
	type S struct {
		_ struct{}      `binary:"endian=big"`
		M [2][2][2]int8 `binary:"[2][2][2]int8"`
	}
	in := S{M: [2][2][2]int8{{{1, 2}, {3, 4}}, {{5, 6}, {7, 8}}}}
	b, err := bst.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := []byte{1, 2, 3, 4, 5, 6, 7, 8}; !bytes.Equal(b, want) {
		t.Fatalf("encode = % x, want % x", b, want)
	}
	var out S
	if _, err := bst.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.M != in.M {
		t.Fatalf("round trip mismatch: %v", out.M)
	}
}

// TestMultidim_FieldRefDims covers field-referenced dimension lengths [R][C]uint8.
func TestMultidim_FieldRefDims(t *testing.T) {
	type S struct {
		_ struct{}  `binary:"endian=big"`
		R uint8     `binary:"uint8"`
		C uint8     `binary:"uint8"`
		M [][]uint8 `binary:"[R][C]uint8"`
	}
	in := S{R: 2, C: 3, M: [][]uint8{{10, 11, 12}, {20, 21, 22}}}
	b, err := bst.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := []byte{2, 3, 10, 11, 12, 20, 21, 22}; !bytes.Equal(b, want) {
		t.Fatalf("encode = % x, want % x", b, want)
	}
	var out S
	if _, err := bst.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.R != 2 || out.C != 3 || len(out.M) != 2 || len(out.M[1]) != 3 || out.M[1][2] != 22 {
		t.Fatalf("round trip wrong: %+v", out)
	}
}

// TestMultidim_NaturalUntagged guards the silent-0 fix: an untagged nested Go
// array must no longer encode to zero bytes.
func TestMultidim_NaturalUntagged(t *testing.T) {
	type S struct {
		_ struct{}    `binary:"endian=big"`
		M [2][3]int16 // no tag
	}
	in := S{M: [2][3]int16{{1, 2, 3}, {4, 5, 6}}}
	b, err := bst.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := []byte{0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6}; !bytes.Equal(b, want) {
		t.Fatalf("natural untagged encode = % x, want % x (silent-0 regression?)", b, want)
	}
	var out S
	if _, err := bst.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.M != in.M {
		t.Fatalf("round trip mismatch: %v", out.M)
	}
}
