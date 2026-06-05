// Copyright 2026 github.com/mixcode

//go:build !safe_binarystruct

package binarystruct

import (
	"bytes"
	"reflect"
	"testing"
)

func TestIsCompatibleFastPath(t *testing.T) {
	tests := []struct {
		goType reflect.Type
		elType eType
		want   bool
	}{
		{reflect.TypeOf(uint32(0)), Uint32, true},
		{reflect.TypeOf(uint32(0)), Dword, true},
		{reflect.TypeOf(int16(0)), Int16, true},
		{reflect.TypeOf(int16(0)), Word, true},
		{reflect.TypeOf(float64(0)), Float64, true},
		// Mismatch size
		{reflect.TypeOf(int64(0)), Int16, false},
		{reflect.TypeOf(int(0)), Int16, false},
		// Mismatch kinds
		{reflect.TypeOf(float32(0)), Uint32, false},
		{reflect.TypeOf(uint32(0)), Float32, false},
		// Non-primitive
		{reflect.TypeOf(""), String, false},
		{reflect.TypeOf(struct{}{}), iStruct, false},
	}

	for _, tt := range tests {
		got := isCompatibleFastPath(tt.goType, tt.elType)
		if got != tt.want {
			t.Errorf("isCompatibleFastPath(%v, %v) = %v; want %v", tt.goType, tt.elType, got, tt.want)
		}
	}
}

func TestUnsafeSliceFastPath(t *testing.T) {
	type testStruct struct {
		Slice []uint32 `binary:"[4]uint32"`
	}

	in := testStruct{
		Slice: []uint32{0x11223344, 0x55667788, 0x99aabbcc, 0xddeeff00},
	}

	// 1. Native / Little Endian (Direct copy)
	blobLE, err := NewMarshalerOrder(LittleEndian).Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal LE failed: %v", err)
	}

	var outLE testStruct
	_, err = NewMarshalerOrder(LittleEndian).Unmarshal(blobLE, &outLE)
	if err != nil {
		t.Fatalf("Unmarshal LE failed: %v", err)
	}

	if !reflect.DeepEqual(in, outLE) {
		t.Errorf("LE mismatch: got %+v, want %+v", outLE, in)
	}

	// 2. Non-Native / Big Endian (Swap bytes)
	blobBE, err := NewMarshalerOrder(BigEndian).Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal BE failed: %v", err)
	}

	// Verify byte swapping happened (e.g. 0x11223344 -> 0x44332211)
	expectedFirstBytes := []byte{0x11, 0x22, 0x33, 0x44}
	if !bytes.Equal(blobBE[:4], expectedFirstBytes) {
		t.Errorf("BE encoding mismatch: got %x, want %x", blobBE[:4], expectedFirstBytes)
	}

	var outBE testStruct
	_, err = NewMarshalerOrder(BigEndian).Unmarshal(blobBE, &outBE)
	if err != nil {
		t.Fatalf("Unmarshal BE failed: %v", err)
	}

	if !reflect.DeepEqual(in, outBE) {
		t.Errorf("BE mismatch: got %+v, want %+v", outBE, in)
	}
}

func TestUnsafeSlicePaddingAndGrowing(t *testing.T) {
	type testStruct struct {
		Slice []uint16 `binary:"[6]uint16"`
	}

	// Slice has fewer elements than tagged size
	in := testStruct{
		Slice: []uint16{0x1122, 0x3344},
	}

	blob, err := NewMarshalerOrder(LittleEndian).Marshal(&in)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Length should be 6 elements * 2 bytes = 12 bytes
	if len(blob) != 12 {
		t.Errorf("Expected 12 bytes, got %d", len(blob))
	}

	// Verify zero padding
	expectedBytes := []byte{0x22, 0x11, 0x44, 0x33, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(blob, expectedBytes) {
		t.Errorf("Mismatched bytes: got %x, want %x", blob, expectedBytes)
	}

	// Unmarshal into a nil slice (must allocate 6 elements)
	var outNil testStruct
	_, err = NewMarshalerOrder(LittleEndian).Unmarshal(blob, &outNil)
	if err != nil {
		t.Fatalf("Unmarshal into nil failed: %v", err)
	}
	expectedSlice := []uint16{0x1122, 0x3344, 0, 0, 0, 0}
	if !reflect.DeepEqual(outNil.Slice, expectedSlice) {
		t.Errorf("Nil slice unmarshal mismatch: got %+v, want %+v", outNil.Slice, expectedSlice)
	}

	// Unmarshal into a slice that is too small (must grow to 6 elements)
	outSmall := testStruct{
		Slice: make([]uint16, 2),
	}
	_, err = NewMarshalerOrder(LittleEndian).Unmarshal(blob, &outSmall)
	if err != nil {
		t.Fatalf("Unmarshal into small slice failed: %v", err)
	}
	if !reflect.DeepEqual(outSmall.Slice, expectedSlice) {
		t.Errorf("Small slice unmarshal mismatch: got %+v, want %+v", outSmall.Slice, expectedSlice)
	}
}
