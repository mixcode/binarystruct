// Copyright 2026 github.com/mixcode

package example

import (
	"bytes"
	"testing"
)

func TestExample(t *testing.T) {
	p := Packet{
		Magic:   "TEST",
		Seq:     12345,
		Version: 2,
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	// Marshal using the generated MarshalBinary method
	blob, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Expected length: 4 (Magic) + 4 (Seq) + 1 (Version) + 8 (Payload) = 17 bytes
	if len(blob) != 17 {
		t.Errorf("expected 17 bytes, got %d", len(blob))
	}

	// Unmarshal using the generated UnmarshalBinary method
	var decoded Packet
	if err := decoded.UnmarshalBinary(blob); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Magic != "TEST" || decoded.Seq != 12345 || decoded.Version != 2 || !bytes.Equal(decoded.Payload, p.Payload) {
		t.Errorf("decoded struct mismatch: %+v", decoded)
	}
}
