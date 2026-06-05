// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"errors"
	"io"
	"testing"
)

func TestUnmarshal_FailureOffset(t *testing.T) {
	type Packet struct {
		Magic uint32 `binary:"uint32"`
		Len   uint16 `binary:"uint16"`
		Data  []byte `binary:"[Len]byte"`
	}

	// Packet with a required length of 10, but payload is truncated at offset 8 (total size should be 16, but input is 8 bytes)
	buf := []byte{
		0xaa, 0xbb, 0xcc, 0xdd, // Magic (4 bytes)
		0x00, 0x0a, // Len = 10 (2 bytes, total 6)
		0x11, 0x22, // Data - only 2 bytes provided (total 8 bytes)
	}

	var p Packet

	_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected error of type *DecodeError, got %T: %v", err, err)
	}

	// Expected offset:
	// Magic is read successfully (4 bytes)
	// Len is read successfully (2 bytes)
	// Data tries to read 10 bytes, but fails at the EOF.
	// The failure offset should be 6 (the start of the Data field).
	if decodeErr.Offset != 6 {
		t.Errorf("expected offset 6, got %d", decodeErr.Offset)
	}
	if decodeErr.Field != "Data" {
		t.Errorf("expected field Data, got %s", decodeErr.Field)
	}
	if !errors.Is(decodeErr.Err, io.ErrUnexpectedEOF) && !errors.Is(decodeErr.Err, io.EOF) {
		t.Errorf("expected EOF or ErrUnexpectedEOF, got %v", decodeErr.Err)
	}
}
