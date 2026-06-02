// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"io"
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
