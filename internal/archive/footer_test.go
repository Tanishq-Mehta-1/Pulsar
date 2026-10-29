package archive

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

func TestFooterSizeContract(t *testing.T) {
	if FooterSize != 24 {
		t.Fatalf("FooterSize = %d, want 24", FooterSize)
	}
	f := Footer{Signature: fzFooterSignature}
	if got := len(SerializeFooter(&f)); got != 24 {
		t.Fatalf("SerializeFooter returned %d bytes, want 24", got)
	}
	// The decoder takes a fixed-size array, so truncated input cannot be
	// expressed; callers must supply exactly FooterSize bytes.
	typ := reflect.TypeOf(DeserializeFooter).In(0)
	if typ.Kind() != reflect.Array || typ.Len() != 24 {
		t.Fatalf("DeserializeFooter parameter is %v, want [24]byte", typ)
	}
}

func TestSerializeFooter_BigEndianLayout(t *testing.T) {
	tests := []struct {
		name string
		f    Footer
		want []byte
	}{
		{
			name: "every field distinct",
			f: Footer{
				Signature:       0xA1B2C3D4,
				DirectoryOffset: 0x0102030405060708,
				DirectorySize:   0x1112131415161718,
				EntryCount:      0x21222324,
			},
			want: []byte{
				0xA1, 0xB2, 0xC3, 0xD4,
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x21, 0x22, 0x23, 0x24,
			},
		},
		{
			name: "valid signature, small values",
			f:    Footer{Signature: fzFooterSignature, DirectoryOffset: 12, DirectorySize: 35, EntryCount: 1},
			want: []byte{
				0x51, 0x61, 0x71, 0x81,
				0, 0, 0, 0, 0, 0, 0, 12,
				0, 0, 0, 0, 0, 0, 0, 35,
				0, 0, 0, 1,
			},
		},
		{
			name: "all zero",
			f:    Footer{},
			want: make([]byte, 24),
		},
		{
			name: "all max",
			f:    Footer{Signature: math.MaxUint32, DirectoryOffset: math.MaxUint64, DirectorySize: math.MaxUint64, EntryCount: math.MaxUint32},
			want: bytes.Repeat([]byte{0xFF}, 24),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SerializeFooter(&tc.f)
			if !bytes.Equal(got[:], tc.want) {
				t.Fatalf("got % x, want % x", got, tc.want)
			}
		})
	}
}

func TestFooterRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		f    Footer
	}{
		{"typical", Footer{Signature: fzFooterSignature, DirectoryOffset: 1234, DirectorySize: 567, EntryCount: 8}},
		{"zero fields", Footer{Signature: fzFooterSignature}},
		{"max fields", Footer{Signature: fzFooterSignature, DirectoryOffset: math.MaxUint64, DirectorySize: math.MaxUint64, EntryCount: math.MaxUint32}},
		{"high bits set", Footer{Signature: fzFooterSignature, DirectoryOffset: 1 << 63, DirectorySize: 1 << 62, EntryCount: 1 << 31}},
		{"offset beyond 32 bits", Footer{Signature: fzFooterSignature, DirectoryOffset: 1<<32 + 5, DirectorySize: 1<<40 + 9, EntryCount: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := SerializeFooter(&tc.f)
			got, err := DeserializeFooter(buf)
			if err != nil {
				t.Fatalf("DeserializeFooter: %v", err)
			}
			if *got != tc.f {
				t.Fatalf("got %+v, want %+v", *got, tc.f)
			}
			if again := SerializeFooter(got); again != buf {
				t.Fatalf("re-serialized bytes differ")
			}
		})
	}
}

func TestDeserializeFooter_DecodesFieldsFromRawBytes(t *testing.T) {
	raw := [FooterSize]byte{
		0x51, 0x61, 0x71, 0x81,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x21, 0x22, 0x23, 0x24,
	}
	got, err := DeserializeFooter(raw)
	if err != nil {
		t.Fatalf("DeserializeFooter: %v", err)
	}
	want := Footer{
		Signature:       0x51617181,
		DirectoryOffset: 0x0102030405060708,
		DirectorySize:   0x1112131415161718,
		EntryCount:      0x21222324,
	}
	if *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
}

func TestDeserializeFooter_RejectsBadSignature(t *testing.T) {
	valid := Footer{Signature: fzFooterSignature, DirectoryOffset: 12, DirectorySize: 0, EntryCount: 0}
	sigs := []struct {
		name string
		sig  uint32
	}{
		{"zero", 0},
		{"max", math.MaxUint32},
		{"off by one", fzFooterSignature + 1},
		{"byte-swapped", 0x81716151},
		{"header signature in footer", fzHeaderSignature},
	}
	for _, tc := range sigs {
		t.Run(tc.name, func(t *testing.T) {
			f := valid
			f.Signature = tc.sig
			got, err := DeserializeFooter(SerializeFooter(&f))
			if err == nil {
				t.Fatalf("expected error, got %+v", got)
			}
			if got != nil {
				t.Fatalf("expected nil footer with error, got %+v", got)
			}
		})
	}

	t.Run("all-zero buffer", func(t *testing.T) {
		if _, err := DeserializeFooter([FooterSize]byte{}); err == nil {
			t.Fatal("expected error")
		}
	})
}
