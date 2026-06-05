// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspect_Basic(t *testing.T) {
	type BasicPacket struct {
		Magic   uint8
		Version uint16 `binary:"uint16,endian=little"`
		Value   uint32
	}

	pkt := BasicPacket{
		Magic:   0xab,
		Version: 0x1234,
		Value:   0xabcdef00,
	}

	layout, err := NewMarshalerOrder(BigEndian).Inspect(pkt)
	if err != nil {
		t.Fatal(err)
	}

	if layout.TypeName != "BasicPacket" {
		t.Errorf("expected TypeName='BasicPacket', got %q", layout.TypeName)
	}
	if len(layout.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(layout.Fields))
	}

	// Magic
	f := layout.Fields[0]
	if f.Name != "Magic" || f.Offset != 0 || f.Size != 1 || f.Endian != "BigEndian" {
		t.Errorf("Magic field layout incorrect: %+v", f)
	}

	// Version
	f = layout.Fields[1]
	if f.Name != "Version" || f.Offset != 1 || f.Size != 2 || f.Endian != "LittleEndian" {
		t.Errorf("Version field layout incorrect: %+v", f)
	}

	// Value
	f = layout.Fields[2]
	if f.Name != "Value" || f.Offset != 3 || f.Size != 4 || f.Endian != "BigEndian" {
		t.Errorf("Value field layout incorrect: %+v", f)
	}

	if layout.TotalSize != 7 {
		t.Errorf("expected total size 7, got %d", layout.TotalSize)
	}
}

func TestInspect_Dynamic(t *testing.T) {
	type DynamicPacket struct {
		HeaderSize uint8
		PayloadLen uint16
		Data       []byte `binary:"[(HeaderSize*2) + PayloadLen]byte"`
	}

	pkt := DynamicPacket{
		HeaderSize: 4,
		PayloadLen: 6,
		Data:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, // size: 4*2 + 6 = 14
	}

	layout, err := NewMarshalerOrder(BigEndian).Inspect(pkt)
	if err != nil {
		t.Fatal(err)
	}

	if len(layout.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(layout.Fields))
	}

	f := layout.Fields[2]
	if f.Name != "Data" || f.Offset != 3 || f.Size != 14 || !strings.Contains(f.Details, "HeaderSize*2") {
		t.Errorf("Data field layout incorrect: %+v", f)
	}
}

func TestInspect_Omittable(t *testing.T) {
	type OmittablePacket struct {
		TotalSize uint16
		Val1      uint32
		Extra1    uint32  `binary:"uint32,omittable=TotalSize"`
		Extra2    *uint32 `binary:"uint32,omittable=TotalSize"`
	}

	// Case 1: All active
	extra2Val := uint32(3)
	pktFull := OmittablePacket{
		TotalSize: 14,
		Val1:      1,
		Extra1:    2,
		Extra2:    &extra2Val,
	}
	layoutFull, err := NewMarshalerOrder(BigEndian).Inspect(pktFull)
	if err != nil {
		t.Fatal(err)
	}
	if len(layoutFull.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(layoutFull.Fields))
	}
	if layoutFull.Fields[2].Size != 4 || layoutFull.Fields[3].Size != 4 {
		t.Errorf("expected Extra1 and Extra2 size 4, got Extra1=%d, Extra2=%d", layoutFull.Fields[2].Size, layoutFull.Fields[3].Size)
	}

	// Case 2: Omitted
	pktOmitted := OmittablePacket{
		TotalSize: 6,
		Val1:      1,
		Extra1:    2,
		Extra2:    nil,
	}
	layoutOmitted, err := NewMarshalerOrder(BigEndian).Inspect(pktOmitted)
	if err != nil {
		t.Fatal(err)
	}
	if len(layoutOmitted.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(layoutOmitted.Fields))
	}
	if layoutOmitted.Fields[2].Size != 0 || !strings.Contains(layoutOmitted.Fields[2].Details, "omitted") {
		t.Errorf("expected Extra1 to be omitted: %+v", layoutOmitted.Fields[2])
	}
	if layoutOmitted.Fields[3].Size != 0 || !strings.Contains(layoutOmitted.Fields[3].Details, "omitted") {
		t.Errorf("expected Extra2 to be omitted: %+v", layoutOmitted.Fields[3])
	}
}

func TestInspect_Nested(t *testing.T) {
	type Sub struct {
		Magic uint16 `binary:"uint16,endian=little"`
	}
	type Parent struct {
		Version uint8
		Info    Sub
	}

	pkt := Parent{
		Version: 1,
		Info: Sub{
			Magic: 0xbeef,
		},
	}

	layout, err := NewMarshalerOrder(BigEndian).Inspect(pkt)
	if err != nil {
		t.Fatal(err)
	}

	if len(layout.Fields) != 2 {
		t.Fatalf("expected 2 flattened fields, got %d", len(layout.Fields))
	}

	// Info.Magic
	f := layout.Fields[1]
	if f.Name != "Info.Magic" || f.Offset != 1 || f.Size != 2 || f.Endian != "LittleEndian" {
		t.Errorf("nested field layout incorrect: %+v", f)
	}
}

func TestInspect_CustomFormatter(t *testing.T) {
	type Simple struct {
		ID uint16
	}
	s := Simple{ID: 255}
	layout, _ := NewMarshalerOrder(BigEndian).Inspect(s)

	// Decimal format
	decStr := layout.String()
	if !strings.Contains(decStr, "OFFSET") || !strings.Contains(decStr, "0 ") || !strings.Contains(decStr, "255") {
		t.Errorf("decimal table incorrect: \n%s", decStr)
	}

	// Hex format
	hexStr := layout.Format(LayoutFormat{OffsetBase: 16, SizeBase: 16, ValueBase: 16})
	if !strings.Contains(hexStr, "0x0") || !strings.Contains(hexStr, "0xff") {
		t.Errorf("hex table incorrect: \n%s", hexStr)
	}
}

func TestInspect_ToJSON(t *testing.T) {
	type Simple struct {
		ID uint16 `binary:"uint16"`
	}
	s := Simple{ID: 255}
	layout, err := NewMarshalerOrder(BigEndian).Inspect(s)
	if err != nil {
		t.Fatal(err)
	}

	js, err := layout.ToJSON()
	if err != nil {
		t.Fatalf("failed to export layout to JSON: %v", err)
	}

	var parsed struct {
		TypeName  string `json:"type_name"`
		TotalSize int    `json:"total_size"`
		Fields    []struct {
			Name       string `json:"name"`
			GoType     string `json:"go_type"`
			BinaryType string `json:"binary_type"`
			Offset     int    `json:"offset"`
			Size       int    `json:"size"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(js, &parsed); err != nil {
		t.Fatalf("failed to parse generated JSON: %v", err)
	}

	if parsed.TypeName != "Simple" || parsed.TotalSize != 2 {
		t.Errorf("incorrect top-level layout values: %+v", parsed)
	}

	if len(parsed.Fields) != 1 || parsed.Fields[0].Name != "ID" || parsed.Fields[0].Offset != 0 || parsed.Fields[0].Size != 2 {
		t.Errorf("incorrect field layout values: %+v", parsed.Fields)
	}
}
