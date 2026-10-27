package archive

import "testing"

func TestSerializeHeader(t *testing.T) {

	//test signature
	header := &Header{
		Signature: fzSignature, // 0x11213141
		Version:   1,
		Flags:     0,
		Reserved:  0,
	}
	expected := [12]byte{
		0x11, 0x21, 0x31, 0x41, // BigEndian uint32
		0x00, 0x01, // BigEndian uint16
		0x00, 0x00, // BigEndian uint16
		0x00, 0x00, 0x00, 0x00, // BigEndian uint32
	}

	got := SerializeHeader(header)
	if got != expected {
		t.Errorf("SerializeHeader() = %x, want %x", got, expected)
	}
}

func TestDeserializeHeader(t *testing.T) {
	tests := []struct {
		name    string
		input   [12]byte
		want    *Header
		wantErr bool
	}{
		{
			name: "valid header",
			input: [12]byte{
				0x11, 0x21, 0x31, 0x41,
				0x00, 0x01,
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			want: &Header{
				Signature: fzSignature,
				Version:   1,
				Flags:     0,
				Reserved:  0,
			},
			wantErr: false,
		},
		{
			name: "invalid signature",
			input: [12]byte{
				0xDE, 0xAD, 0xBE, 0xEF,
				0x00, 0x01,
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "unsupported version",
			input: [12]byte{
				0x11, 0x21, 0x31, 0x41,
				0x00, 0x02, // version 2
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "non-zero flags error",
			input: [12]byte{
				0x11, 0x21, 0x31, 0x41,
				0x00, 0x01,
				0x00, 0x01, // flags != 0
				0x00, 0x00, 0x00, 0x00,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "non-zero reserved error",
			input: [12]byte{
				0x11, 0x21, 0x31, 0x41,
				0x00, 0x01,
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x01, // reserved != 0
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeserializeHeader(tt.input)

			if (err != nil) != tt.wantErr {
				t.Fatalf("DeserializeHeader() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if *got != *tt.want {
					t.Errorf("DeserializeHeader() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := &Header{
		Signature: fzSignature,
		Version:   1,
		Flags:     0,
		Reserved:  0,
	}

	buf := SerializeHeader(original)
	got, err := DeserializeHeader(buf)
	if err != nil {
		t.Fatalf("unexpected error during roundtrip: %v", err)
	}

	if *got != *original {
		t.Errorf("Roundtrip result = %+v, want %+v", got, original)
	}
}
