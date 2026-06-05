// Copyright 2026 github.com/mixcode

//go:build safe_binarystruct

package binarystruct

import (
	"io"
	"reflect"
)

const safeMode = true

func (ms *Marshaler) unsafeWriteStruct(w io.Writer, order ByteOrder, strc reflect.Value) (n int, err error) {
	panic("unsafeWriteStruct called in safe mode")
}

func (ms *Marshaler) unsafeReadStruct(r io.Reader, order ByteOrder, strc reflect.Value) (n int, err error) {
	panic("unsafeReadStruct called in safe mode")
}
