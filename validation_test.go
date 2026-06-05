// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"errors"
	"testing"
)

func TestValidation_Range(t *testing.T) {
	type Packet struct {
		Val uint16 `binary:"uint16,range=1..100"`
	}

	// 1. Valid case
	{
		buf := []byte{0x00, 0x32} // 50
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatalf("unexpected error for valid value: %v", err)
		}
		if p.Val != 50 {
			t.Errorf("expected 50, got %d", p.Val)
		}
	}

	// 2. Out of range case
	{
		buf := []byte{0x00, 0xc8} // 200
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) {
			t.Fatalf("expected DecodeError, got %T: %v", err, err)
		}
		if !errors.Is(decodeErr.Err, ErrValidationError) {
			t.Errorf("expected ErrValidationError, got %v", decodeErr.Err)
		}
		if decodeErr.Field != "Val" {
			t.Errorf("expected field Val, got %s", decodeErr.Field)
		}
	}
}

func TestValidation_OpenRange(t *testing.T) {
	type Packet struct {
		Val int8 `binary:"int8,range=0.."`
	}

	// 1. Valid non-negative
	{
		buf := []byte{10}
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Val != 10 {
			t.Errorf("expected 10, got %d", p.Val)
		}
	}

	// 2. Invalid negative
	{
		buf := []byte{0xf6} // -10
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || !errors.Is(decodeErr.Err, ErrValidationError) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}

func TestValidation_RegexMatch(t *testing.T) {
	type Packet struct {
		Code string `binary:"string(4),match=^[A-Z]+$"`
	}

	// 1. Valid uppercase string
	{
		buf := []byte("ABCD")
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Code != "ABCD" {
			t.Errorf("expected ABCD, got %s", p.Code)
		}
	}

	// 2. Invalid lowercase string
	{
		buf := []byte("abcd")
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || !errors.Is(decodeErr.Err, ErrValidationError) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}

func TestValidation_Array(t *testing.T) {
	type Packet struct {
		Scores []uint8 `binary:"[3]uint8,range=1..10"`
	}

	// 1. Valid array
	{
		buf := []byte{1, 5, 10}
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// 2. Invalid array element
	{
		buf := []byte{1, 15, 10}
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) || !errors.Is(decodeErr.Err, ErrValidationError) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}
