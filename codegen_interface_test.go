// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

type mockGeneratedStruct struct {
	Val         uint32
	calledWrite bool
	calledRead  bool
}

func (m *mockGeneratedStruct) WriteBinary(w io.Writer, order bst.ByteOrder) (int, error) {
	m.calledWrite = true
	var buf [4]byte
	order.PutUint32(buf[:], m.Val)
	return w.Write(buf[:])
}

func (m *mockGeneratedStruct) ReadBinary(r io.Reader, order bst.ByteOrder) (int, error) {
	m.calledRead = true
	var buf [4]byte
	n, err := io.ReadFull(r, buf[:])
	if err != nil {
		return n, err
	}
	m.Val = order.Uint32(buf[:])
	return n, nil
}

func TestCodegenInterface_FastPath(t *testing.T) {
	s := mockGeneratedStruct{Val: 0xDEADBEEF}
	blob, err := bst.Marshal(&s, bst.BigEndian)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !s.calledWrite {
		t.Error("expected WriteBinary to be called")
	}
	expected := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(blob, expected) {
		t.Errorf("expected %x, got %x", expected, blob)
	}

	var s2 mockGeneratedStruct
	_, err = bst.Unmarshal(blob, bst.BigEndian, &s2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !s2.calledRead {
		t.Error("expected ReadBinary to be called")
	}
	if s2.Val != 0xDEADBEEF {
		t.Errorf("expected 0xDEADBEEF, got %x", s2.Val)
	}
}

type mockContextStruct struct {
	Val         string
	calledWrite bool
	calledRead  bool
}

func (m *mockContextStruct) WriteBinaryWithMarshaller(ms *bst.Marshaller, w io.Writer, order bst.ByteOrder) (int, error) {
	m.calledWrite = true
	return w.Write([]byte(m.Val))
}

func (m *mockContextStruct) ReadBinaryWithMarshaller(ms *bst.Marshaller, r io.Reader, order bst.ByteOrder) (int, error) {
	m.calledRead = true
	var buf [5]byte
	n, err := io.ReadFull(r, buf[:])
	if err != nil {
		return n, err
	}
	m.Val = string(buf[:])
	return n, nil
}

func TestCodegenContextInterface_FastPath(t *testing.T) {
	s := mockContextStruct{Val: "hello"}
	blob, err := bst.Marshal(&s, bst.BigEndian)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !s.calledWrite {
		t.Error("expected WriteBinaryWithMarshaller to be called")
	}
	if string(blob) != "hello" {
		t.Errorf("expected hello, got %s", string(blob))
	}

	var s2 mockContextStruct
	_, err = bst.Unmarshal(blob, bst.BigEndian, &s2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !s2.calledRead {
		t.Error("expected ReadBinaryWithMarshaller to be called")
	}
	if s2.Val != "hello" {
		t.Errorf("expected hello, got %s", s2.Val)
	}
}

// mockSerializer is a minimal Serializer for testing GetSerializer.
type mockSerializer struct{}

func (mockSerializer) Serialize(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, order bst.ByteOrder) (int, error) {
	return 0, nil
}
func (mockSerializer) Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, order bst.ByteOrder) (interface{}, int, error) {
	return nil, 0, nil
}

func TestGetSerializer(t *testing.T) {
	var ms bst.Marshaller

	// GetSerializer on empty Marshaller should return nil
	if s := ms.GetSerializer("foo"); s != nil {
		t.Error("expected nil for unregistered serializer on empty Marshaller")
	}

	// Register and retrieve
	ms.AddSerializer("myser", mockSerializer{})
	if s := ms.GetSerializer("myser"); s == nil {
		t.Error("expected non-nil for registered serializer")
	}

	// Not found after registration of a different name
	if s := ms.GetSerializer("other"); s != nil {
		t.Error("expected nil for unregistered name")
	}

	// Remove and verify gone
	ms.RemoveSerializer("myser")
	if s := ms.GetSerializer("myser"); s != nil {
		t.Error("expected nil after removal")
	}
}
