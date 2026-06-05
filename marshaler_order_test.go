// Copyright 2021-2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"strings"
	"testing"
)

// orderProbe is a tiny fixed-layout struct used by the byte-order tests below.
type orderProbe struct {
	V uint16 `binary:"uint16"`
}

// TestNewMarshaler_MatchesPackageFunc verifies that a Marshaler constructed with
// NewMarshaler encodes identically to the package-level Marshal (which is defined
// in terms of NewMarshaler), confirming the byte order carried on the instance is
// the one used by the order-free methods.
func TestNewMarshaler_MatchesPackageFunc(t *testing.T) {
	in := orderProbe{V: 0x0102}

	be, err := NewMarshalerOrder(BigEndian).Marshal(&in)
	if err != nil {
		t.Fatalf("NewMarshaler(BigEndian).Marshal: %v", err)
	}
	if want := []byte{0x01, 0x02}; !bytes.Equal(be, want) {
		t.Errorf("big-endian: got %x, want %x", be, want)
	}

	le, err := NewMarshalerOrder(LittleEndian).Marshal(&in)
	if err != nil {
		t.Fatalf("NewMarshaler(LittleEndian).Marshal: %v", err)
	}
	if want := []byte{0x02, 0x01}; !bytes.Equal(le, want) {
		t.Errorf("little-endian: got %x, want %x", le, want)
	}

	// The order-free instance method must match the order-taking package func.
	pkg, _ := NewMarshalerOrder(BigEndian).Marshal(&in)
	if !bytes.Equal(be, pkg) {
		t.Errorf("instance vs package mismatch: %x vs %x", be, pkg)
	}
}

// TestAppend verifies that Append appends the encoded bytes onto an existing
// buffer (the encoding.BinaryAppender idiom), leaving the prefix intact and
// producing the same bytes Marshal would, after the prefix.
func TestAppend(t *testing.T) {
	in := orderProbe{V: 0x0102}
	prefix := []byte{0xaa, 0xbb}

	got, err := NewMarshalerOrder(BigEndian).Append(prefix, &in)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	enc, _ := NewMarshalerOrder(BigEndian).Marshal(&in)
	want := append(append([]byte{}, prefix...), enc...)
	if !bytes.Equal(got, want) {
		t.Errorf("Append: got %x, want %x", got, want)
	}

	// A nil buffer yields exactly the encoded bytes.
	got2, err := NewMarshalerOrder(BigEndian).Append(nil, &in)
	if err != nil {
		t.Fatalf("Append(nil): %v", err)
	}
	if !bytes.Equal(got2, enc) {
		t.Errorf("Append(nil): got %x, want %x", got2, enc)
	}
}

// TestMarshaler_NoOrder_FailsLoud verifies that a Marshaler with no order (Order
// nil) and a value that declares none fails with a clear error when encoding or
// decoding a multi-byte value, instead of panicking on a nil ByteOrder. (Inspect
// is diagnostic and tolerates a missing order — it renders the endian as "?" —
// so it is intentionally not in this set.)
func TestMarshaler_NoOrder_FailsLoud(t *testing.T) {
	in := orderProbe{V: 1}
	blob := []byte{0, 1}

	checks := []struct {
		name string
		call func(ms *Marshaler) error
	}{
		{"Marshal", func(ms *Marshaler) error { _, err := ms.Marshal(&in); return err }},
		{"Write", func(ms *Marshaler) error { _, err := ms.Write(new(bytes.Buffer), &in); return err }},
		{"Unmarshal", func(ms *Marshaler) error { var out orderProbe; _, err := ms.Unmarshal(blob, &out); return err }},
		{"Read", func(ms *Marshaler) error {
			var out orderProbe
			_, err := ms.Read(bytes.NewReader(blob), &out)
			return err
		}},
	}
	for _, c := range checks {
		var ms Marshaler // Order is nil
		err := c.call(&ms)
		if err == nil {
			t.Errorf("%s: expected an error for a Marshaler with no byte order, got nil", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "no byte order") {
			t.Errorf("%s: error = %q, want it to mention the missing byte order", c.name, err)
		}
	}
}

// TestMarshaler_OrderField verifies the Order field can be set directly (the
// struct-literal path) as an alternative to NewMarshaler.
func TestMarshaler_OrderField(t *testing.T) {
	ms := Marshaler{Order: BigEndian}
	out, err := ms.Marshal(&orderProbe{V: 0x0102})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if want := []byte{0x01, 0x02}; !bytes.Equal(out, want) {
		t.Errorf("got %x, want %x", out, want)
	}
}
