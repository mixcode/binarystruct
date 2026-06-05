// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"testing"
)

func TestOmittable_EOF_Scalar(t *testing.T) {
	type Packet struct {
		Required uint16
		Optional uint32 `binary:"uint32,omittable"`
	}

	// Case 1: Full payload
	{
		buf := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x02}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 6 {
			t.Errorf("expected 6 bytes read, got %d", n)
		}
		if p.Required != 1 || p.Optional != 2 {
			t.Errorf("expected Required=1, Optional=2, got Required=%d, Optional=%d", p.Required, p.Optional)
		}
	}

	// Case 2: Omitted trailing field
	{
		buf := []byte{0x00, 0x01}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("expected 2 bytes read, got %d", n)
		}
		if p.Required != 1 || p.Optional != 0 {
			t.Errorf("expected Required=1, Optional=0, got Required=%d, Optional=%d", p.Required, p.Optional)
		}
	}

	// Case 3: Partial read (should error)
	{
		buf := []byte{0x00, 0x01, 0x00, 0x00}
		var p Packet
		_, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err == nil {
			t.Fatal("expected error on partial read of optional field")
		}
	}
}

func TestOmittable_EOF_Pointer(t *testing.T) {
	type Packet struct {
		Required uint16
		Optional *uint32 `binary:"uint32,omittable"`
	}

	// Case 1: Full payload
	{
		buf := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x02}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 6 {
			t.Errorf("expected 6 bytes read, got %d", n)
		}
		if p.Required != 1 || p.Optional == nil || *p.Optional != 2 {
			t.Errorf("expected Required=1, Optional=2, got Required=%d, Optional=%v", p.Required, p.Optional)
		}
	}

	// Case 2: Omitted trailing field (pointer should be nil)
	{
		buf := []byte{0x00, 0x01}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("expected 2 bytes read, got %d", n)
		}
		if p.Required != 1 || p.Optional != nil {
			t.Errorf("expected Required=1, Optional=nil, got Required=%d, Optional=%v", p.Required, p.Optional)
		}
	}
}

func TestOmittable_Expression(t *testing.T) {
	type Packet struct {
		TotalSize uint16
		Val1      uint32
		Extra1    uint32  `binary:"uint32,omittable=TotalSize"`
		Extra2    *uint32 `binary:"uint32,omittable=TotalSize"`
	}

	// Case 1: TotalSize = 6 (both Extra1 and Extra2 omitted)
	{
		buf := []byte{
			0x00, 0x06, // TotalSize = 6
			0x00, 0x00, 0x00, 0x01, // Val1
			0x00, 0x00, 0x00, 0x02, // Extra1 (should be ignored)
		}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 6 {
			t.Errorf("expected 6 bytes read, got %d", n)
		}
		if p.Val1 != 1 || p.Extra1 != 0 || p.Extra2 != nil {
			t.Errorf("expected Val1=1, Extra1=0, Extra2=nil, got Val1=%d, Extra1=%d, Extra2=%v", p.Val1, p.Extra1, p.Extra2)
		}
	}

	// Case 2: TotalSize = 10 (Extra1 present, Extra2 omitted)
	{
		buf := []byte{
			0x00, 0x0a, // TotalSize = 10
			0x00, 0x00, 0x00, 0x01, // Val1
			0x00, 0x00, 0x00, 0x02, // Extra1
			0x00, 0x00, 0x00, 0x03, // Extra2 (should be ignored)
		}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 10 {
			t.Errorf("expected 10 bytes read, got %d", n)
		}
		if p.Val1 != 1 || p.Extra1 != 2 || p.Extra2 != nil {
			t.Errorf("expected Val1=1, Extra1=2, Extra2=nil, got Val1=%d, Extra1=%d, Extra2=%v", p.Val1, p.Extra1, p.Extra2)
		}
	}

	// Case 3: TotalSize = 14 (Extra1 and Extra2 both present)
	{
		buf := []byte{
			0x00, 0x0e, // TotalSize = 14
			0x00, 0x00, 0x00, 0x01, // Val1
			0x00, 0x00, 0x00, 0x02, // Extra1
			0x00, 0x00, 0x00, 0x03, // Extra2
		}
		var p Packet
		n, err := NewMarshalerOrder(BigEndian).Unmarshal(buf, &p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 14 {
			t.Errorf("expected 14 bytes read, got %d", n)
		}
		if p.Val1 != 1 || p.Extra1 != 2 || p.Extra2 == nil || *p.Extra2 != 3 {
			t.Errorf("expected Val1=1, Extra1=2, Extra2=3, got Val1=%d, Extra1=%d, Extra2=%v", p.Val1, p.Extra1, p.Extra2)
		}
	}
}

func TestOmittable_Marshal(t *testing.T) {
	type Packet struct {
		TotalSize uint16
		Val1      uint32
		Extra1    uint32  `binary:"uint32,omittable=TotalSize"`
		Extra2    *uint32 `binary:"uint32,omittable=TotalSize"`
	}

	// Case 1: Extra2 is nil (should omit Extra2)
	{
		extra1 := uint32(2)
		p := Packet{
			TotalSize: 10,
			Val1:      1,
			Extra1:    extra1,
			Extra2:    nil,
		}
		blob, err := NewMarshalerOrder(BigEndian).Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		expected := []byte{
			0x00, 0x0a,
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x00, 0x00, 0x02,
		}
		if !bytes.Equal(blob, expected) {
			t.Errorf("expected %x, got %x", expected, blob)
		}
	}

	// Case 2: TotalSize is set to 6 (should omit both Extra1 and Extra2 based on expression)
	{
		extra2 := uint32(3)
		p := Packet{
			TotalSize: 6,
			Val1:      1,
			Extra1:    2,
			Extra2:    &extra2,
		}
		blob, err := NewMarshalerOrder(BigEndian).Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		expected := []byte{
			0x00, 0x06,
			0x00, 0x00, 0x00, 0x01,
		}
		if !bytes.Equal(blob, expected) {
			t.Errorf("expected %x, got %x", expected, blob)
		}
	}
}
