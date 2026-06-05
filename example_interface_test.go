// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"
	"io"
	"reflect"

	"github.com/mixcode/binarystruct"
)

// Define polymorphic structures
type MessageA struct {
	ValueA uint32 `binary:"uint32"`
}

type MessageB struct {
	ValueB string `binary:"string(8)"`
}

// Packet wraps a dynamic payload
type Packet struct {
	MsgType uint8       `binary:"uint8"`
	Payload interface{} `binary:"custom,serializer=DynamicPayload"`
}

// DynamicPayloadSerializer handles dynamic allocation
type DynamicPayloadSerializer struct{}

func (s *DynamicPayloadSerializer) Serialize(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (n int, err error) {
	// Serialization simply delegates to standard Write
	return binarystruct.Write(w, order, value)
}

func (s *DynamicPayloadSerializer) Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
	// 1. Inspect the previously decoded "MsgType" field in the parent struct
	msgTypeField := parentStruct.FieldByName("MsgType")
	if !msgTypeField.IsValid() {
		return nil, 0, fmt.Errorf("MsgType field not found in parent struct")
	}

	// 2. Allocate the appropriate structure dynamically
	var payload interface{}
	switch msgTypeField.Uint() {
	case 1:
		payload = &MessageA{}
	case 2:
		payload = &MessageB{}
	default:
		return nil, 0, fmt.Errorf("unknown message type: %d", msgTypeField.Uint())
	}

	// 3. Decode binary stream into the allocated structure
	n, err = binarystruct.Read(r, order, payload)
	if err != nil {
		return nil, n, err
	}

	return payload, n, nil
}

func Example_interfacePolymorphism() {
	// Register the custom serializer
	var ms binarystruct.Marshaler
	ms.Order = binarystruct.BigEndian
	ms.AddSerializer("DynamicPayload", &DynamicPayloadSerializer{})

	// 1. Serialize Packet A (MsgType 1)
	pktA := Packet{
		MsgType: 1,
		Payload: &MessageA{ValueA: 0x11223344},
	}
	blobA, _ := ms.Marshal(&pktA)

	// 2. Serialize Packet B (MsgType 2)
	pktB := Packet{
		MsgType: 2,
		Payload: &MessageB{ValueB: "hello"},
	}
	blobB, _ := ms.Marshal(&pktB)

	// 3. Deserialize back dynamically
	var restoredA Packet
	_, _ = ms.Unmarshal(blobA, &restoredA)

	var restoredB Packet
	_, _ = ms.Unmarshal(blobB, &restoredB)

	// Verify types and values
	fmt.Printf("Restored A Payload: %T (%+v)\n", restoredA.Payload, restoredA.Payload)
	fmt.Printf("Restored B Payload: %T (%+v)\n", restoredB.Payload, restoredB.Payload)

	// Output:
	// Restored A Payload: *binarystruct_test.MessageA (&{ValueA:287454020})
	// Restored B Payload: *binarystruct_test.MessageB (&{ValueB:hello})
}
