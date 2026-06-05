// Copyright 2021-2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"reflect"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
)

// "あ" (U+3042): UTF-8 = e3 81 82, Shift-JIS = 82 a0, UTF-16BE = 30 42 — so the
// chosen text encoding is visible in the bytes. wstring writes a 2-byte (here
// big-endian) length prefix, then the encoded content.

// TestStructLevelEncoding_Applies: the sentinel's encoding= is used by string
// fields that declare none, and round-trips. It also beats the Marshaler-wide
// DefaultTextEncoding.
func TestStructLevelEncoding_Applies(t *testing.T) {
	type S struct {
		_    struct{} `binary:"endian=big,encoding=sjis"`
		Text string   `binary:"wstring"`
	}
	ms := NewMarshaler()
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)
	ms.AddTextEncoding("utf16", unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM))
	ms.DefaultTextEncoding = "utf16" // the struct's sjis must win over this

	blob, err := ms.Marshal(&S{Text: "あ"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x00, 0x02, 0x82, 0xa0}; !bytes.Equal(blob, want) {
		t.Errorf("got %x, want %x (struct-level sjis, not the marshaler's utf16)", blob, want)
	}

	var out S
	if _, err := ms.Unmarshal(blob, &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != "あ" {
		t.Errorf("round-trip Text = %q, want あ", out.Text)
	}
}

// TestStructLevelEncoding_FieldOverrides: a per-field encoding= overrides the
// struct-level default for that field only.
func TestStructLevelEncoding_FieldOverrides(t *testing.T) {
	type S struct {
		_ struct{} `binary:"endian=big,encoding=sjis"`
		A string   `binary:"wstring"`                // struct default → sjis
		B string   `binary:"wstring,encoding=utf16"` // overrides → utf16
	}
	ms := NewMarshaler()
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)
	ms.AddTextEncoding("utf16", unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM))

	blob, err := ms.Marshal(&S{A: "あ", B: "あ"})
	if err != nil {
		t.Fatal(err)
	}
	// A: sjis 82 a0 ; B: utf16-BE 30 42 (each with a 2-byte big-endian length prefix)
	want := []byte{0x00, 0x02, 0x82, 0xa0, 0x00, 0x02, 0x30, 0x42}
	if !bytes.Equal(blob, want) {
		t.Errorf("got %x, want %x (A sjis, B utf16)", blob, want)
	}
}

// TestStructLevelEncoding_Embedded: a base struct declaring endian+encoding
// propagates both via embedding, so no order argument or per-field encoding is
// needed.
func TestStructLevelEncoding_Embedded(t *testing.T) {
	type sjisBE struct {
		_ struct{} `binary:"endian=big,encoding=sjis"`
	}
	type Msg struct {
		sjisBE
		Text string `binary:"wstring"`
	}
	ms := NewMarshaler() // no order: the embedded base supplies endian=big
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)

	blob, err := ms.Marshal(&Msg{Text: "あ"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x00, 0x02, 0x82, 0xa0}; !bytes.Equal(blob, want) {
		t.Errorf("got %x, want %x (endian+encoding inherited from the embedded base)", blob, want)
	}
}

// TestStructLevelEncoding_Metadata: the sentinel's encoding is recorded and baked
// into string fields, but not onto non-string fields.
func TestStructLevelEncoding_Metadata(t *testing.T) {
	type S struct {
		_ struct{} `binary:"encoding=sjis"`
		N uint16   `binary:"uint16"`
		T string   `binary:"wstring"`
	}
	m, err := getStructMetadata(reflect.TypeOf(S{}))
	if err != nil {
		t.Fatal(err)
	}
	if m.defaultEncoding != "sjis" {
		t.Errorf("defaultEncoding = %q, want sjis", m.defaultEncoding)
	}
	for _, f := range m.fields {
		switch f.name {
		case "N":
			if f.encoding != "" {
				t.Errorf("non-string field N got encoding %q, want none", f.encoding)
			}
		case "T":
			if f.encoding != "sjis" {
				t.Errorf("string field T encoding = %q, want sjis (baked from the struct default)", f.encoding)
			}
		}
	}
}
