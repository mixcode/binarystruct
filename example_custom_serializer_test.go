// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/mixcode/binarystruct"
)

// VarintCodec encodes and decodes integer values as 7-bit varints.
type VarintCodec struct{}

func (vs *VarintCodec) Encode(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (n int, err error) {
	v, ok := value.(int)
	if !ok {
		return 0, fmt.Errorf("expected int")
	}
	var buf [10]byte
	length := binary.PutUvarint(buf[:], uint64(v))
	return w.Write(buf[:length])
}

func (vs *VarintCodec) Decode(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
	var val uint64
	var shift uint
	var bytesRead int
	if br, ok := r.(io.ByteReader); ok {
		for {
			b, err := br.ReadByte()
			if err != nil {
				return nil, bytesRead, err
			}
			bytesRead++
			val |= uint64(b&0x7f) << shift
			if b&0x80 == 0 {
				break
			}
			shift += 7
		}
	} else {
		var b [1]byte
		for {
			_, err := r.Read(b[:])
			if err != nil {
				return nil, bytesRead, err
			}
			bytesRead++
			val |= uint64(b[0]&0x7f) << shift
			if b[0]&0x80 == 0 {
				break
			}
			shift += 7
		}
	}
	return int(val), bytesRead, nil
}

func ExampleMarshaler_AddCodec() {
	marshaller := new(binarystruct.Marshaler)
	marshaller.Order = binarystruct.LittleEndian
	marshaller.AddCodec("varint", &VarintCodec{})

	type Packet struct {
		// Custom Codec: uses the registered "varint" codec
		Count int `binary:"custom,codec=varint"`
	}

	in := Packet{
		Count: 300,
	}

	// Marshal structural data
	blob, err := marshaller.Marshal(in)
	if err != nil {
		panic(err)
	}

	// 300 in varint is [0xac, 0x02]
	fmt.Printf("Blob: %x\n", blob)

	var restored Packet
	_, err = marshaller.Unmarshal(blob, &restored)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restored: Count=%d\n", restored.Count)

	// Output:
	// Blob: ac02
	// Restored: Count=300
}
