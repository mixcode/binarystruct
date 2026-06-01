// Copyright 2026 github.com/mixcode

//go:build safe

package binarystruct

import (
	"io"
	"reflect"
)

const safeMode = true

func (ms *Marshaller) unsafeWriteStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	panic("unsafeWriteStruct called in safe mode")
}

func (ms *Marshaller) unsafeReadStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
	panic("unsafeReadStruct called in safe mode")
}
