package archive

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

func TestHeaderSizeContract(t *testing.T) {
	if HeaderSize != 12 {
		t.Fatalf("HeaderSize = %d, want 12", HeaderSize)
	}
	h := Header{Signature: fzHeaderSignature, Version: 1}
	if got := len(SerializeHeader(&h)); got != 12 {
		t.Fatalf("SerializeHeader returned %d bytes, want 12", got)
	}
	// The decoder takes a fixed-size array, so truncated input cannot be
	// expressed; callers must supply exactly HeaderSize bytes.
	typ := reflect.TypeOf(DeserializeHeader).In(0)
	if typ.Kind() != reflect.Array || typ.Len() != 12 {
		t.Fatalf("DeserializeHeader parameter is %v, want [12]byte", typ)
	}
}

func TestSerializeHeader_BigEndianLayout(t *testing.T) {
	tests := []struct {
		name string
		h    Header
		want []byte
	}{
		{
			name: "every field distinct",
			h:    Header{Signature: 0xA1B2C3D4, Version: 0x0102, Flags: 0x0304, Reserved: 0x05060708},
			want: []byte{0xA1, 0xB2, 0xC3, 0xD4, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		},
		{
			name: "valid header",
			h:    Header{Signature: fzHeaderSignature, Version: 1},
			want: []byte{0x11, 0x21, 0x31, 0x41, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "all zero",
			h:    Header{},
			want: make([]byte, 12),
		},
		{
			name: "all max",
			h:    Header{Signature: math.MaxUint32, Version: math.MaxUint16, Flags: math.MaxUint16, Reserved: math.MaxUint32},
			want: bytes.Repeat([]byte{0xFF}, 12),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SerializeHeader(&tc.h)
			if !bytes.Equal(got[:], tc.want) {
				t.Fatalf("got % x, want % x", got, tc.want)
			}
		})
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	in := Header{Signature: fzHeaderSignature, Version: 1, Flags: 0, Reserved: 0}
	buf := SerializeHeader(&in)
	out, err := DeserializeHeader(buf)
	if err != nil {
		t.Fatalf("DeserializeHeader: %v", err)
	}
	if *out != in {
		t.Fatalf("round trip mismatch: got %+v, want %+v", *out, in)
	}
	// Serializing the decoded value must reproduce the original bytes.
	again := SerializeHeader(out)
	if again != buf {
		t.Fatalf("re-serialized bytes differ: % x vs % x", again, buf)
	}
}

func TestDeserializeHeader_Rejects(t *testing.T) {
	valid := Header{Signature: fzHeaderSignature, Version: 1}
	mk := func(mutate func(*Header)) Header {
		h := valid
		mutate(&h)
		return h
	}
	tests := []struct {
		name string
		h    Header
	}{
		{"signature zero", mk(func(h *Header) { h.Signature = 0 })},
		{"signature max", mk(func(h *Header) { h.Signature = math.MaxUint32 })},
		{"signature off by one", mk(func(h *Header) { h.Signature = fzHeaderSignature + 1 })},
		{"signature byte-swapped", mk(func(h *Header) { h.Signature = 0x41312111 })},
		{"footer signature in header", mk(func(h *Header) { h.Signature = fzFooterSignature })},
		{"version zero", mk(func(h *Header) { h.Version = 0 })},
		{"version two", mk(func(h *Header) { h.Version = 2 })},
		{"version max", mk(func(h *Header) { h.Version = math.MaxUint16 })},
		{"version byte-swapped", mk(func(h *Header) { h.Version = 0x0100 })},
		{"flags one", mk(func(h *Header) { h.Flags = 1 })},
		{"flags high bit", mk(func(h *Header) { h.Flags = 0x8000 })},
		{"flags max", mk(func(h *Header) { h.Flags = math.MaxUint16 })},
		{"reserved one", mk(func(h *Header) { h.Reserved = 1 })},
		{"reserved high bit", mk(func(h *Header) { h.Reserved = 0x80000000 })},
		{"reserved max", mk(func(h *Header) { h.Reserved = math.MaxUint32 })},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeserializeHeader(SerializeHeader(&tc.h))
			if err == nil {
				t.Fatalf("expected error, got header %+v", got)
			}
			if got != nil {
				t.Fatalf("expected nil header alongside error, got %+v", got)
			}
		})
	}

	t.Run("all-zero buffer", func(t *testing.T) {
		if _, err := DeserializeHeader([HeaderSize]byte{}); err == nil {
			t.Fatal("expected error for zero buffer")
		}
	})

	t.Run("little-endian encoded valid header", func(t *testing.T) {
		le := [HeaderSize]byte{0x41, 0x31, 0x21, 0x11, 0x01, 0x00}
		if _, err := DeserializeHeader(le); err == nil {
			t.Fatal("expected error for little-endian encoding")
		}
	})
}
