// Copyright 2021-2026 github.com/mixcode

package binarystruct

import (
	"reflect"
	"testing"
)

// beBase / leBase are reusable byte-order marker bases: an embeddable struct that
// declares an order via the blank `_` sentinel and encodes to zero bytes.
type beBase struct {
	_ struct{} `binary:"endian=big"`
}
type leBase struct {
	_ struct{} `binary:"endian=little"`
}

// TestStructLevelEndian_Sentinel: a blank `_ struct{}` sentinel sets the struct's
// byte order and is excluded from the encoded layout.
func TestStructLevelEndian_Sentinel(t *testing.T) {
	type S struct {
		_ struct{} `binary:"endian=big"`
		A uint16   `binary:"uint16"`
		B uint16   `binary:"uint16"`
	}
	m, err := getStructMetadata(reflect.TypeOf(S{}))
	if err != nil {
		t.Fatal(err)
	}
	if m.endian != endianBig {
		t.Errorf("struct endian = %v, want endianBig", m.endian)
	}
	if len(m.fields) != 2 {
		t.Errorf("fields = %d, want 2 (sentinel excluded)", len(m.fields))
	}
	for _, f := range m.fields {
		if f.name == "_" {
			t.Errorf("the `_` sentinel leaked into the encoded layout")
		}
	}
}

// TestStructLevelEndian_None: a struct without a sentinel declares no order.
func TestStructLevelEndian_None(t *testing.T) {
	type S struct {
		A uint16 `binary:"uint16"`
	}
	m, err := getStructMetadata(reflect.TypeOf(S{}))
	if err != nil {
		t.Fatal(err)
	}
	if m.endian != endianNone {
		t.Errorf("struct endian = %v, want endianNone", m.endian)
	}
}

// TestStructLevelEndian_EmbeddedPropagates: embedding a marker base inherits its order.
func TestStructLevelEndian_EmbeddedPropagates(t *testing.T) {
	type S struct {
		beBase
		A uint16 `binary:"uint16"`
	}
	m, err := getStructMetadata(reflect.TypeOf(S{}))
	if err != nil {
		t.Fatal(err)
	}
	if m.endian != endianBig {
		t.Errorf("struct endian = %v, want endianBig (inherited from beBase)", m.endian)
	}
}

// TestStructLevelEndian_OwnWinsOverEmbedded: a local sentinel overrides an embedded order.
func TestStructLevelEndian_OwnWinsOverEmbedded(t *testing.T) {
	type S struct {
		beBase
		_ struct{} `binary:"endian=little"`
		A uint16   `binary:"uint16"`
	}
	m, err := getStructMetadata(reflect.TypeOf(S{}))
	if err != nil {
		t.Fatal(err)
	}
	if m.endian != endianLittle {
		t.Errorf("struct endian = %v, want endianLittle (own sentinel wins)", m.endian)
	}
}

// TestStructLevelEndian_ConflictingEmbedsError: two embeds with different orders is an error.
func TestStructLevelEndian_ConflictingEmbedsError(t *testing.T) {
	type S struct {
		beBase
		leBase
		A uint16 `binary:"uint16"`
	}
	if _, err := getStructMetadata(reflect.TypeOf(S{})); err == nil {
		t.Fatal("expected an error for conflicting embedded byte orders, got nil")
	}
}

// TestStructLevelEndian_UnknownOptionError: an unknown struct-level option fails loud.
func TestStructLevelEndian_UnknownOptionError(t *testing.T) {
	type S struct {
		_ struct{} `binary:"frobnicate=1"`
		A uint16   `binary:"uint16"`
	}
	if _, err := getStructMetadata(reflect.TypeOf(S{})); err == nil {
		t.Fatal("expected an error for an unknown struct-level option, got nil")
	}
}
