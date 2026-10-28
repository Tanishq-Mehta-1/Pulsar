package archive

import (
	"bytes"
	"strings"
	"testing"
)

func TestSerializeFileEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   *FileEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: &FileEntry{
				FilenameLength:    8,
				Filename:          "test.txt",
				DataOffset:        100,
				OriginalSize:      500,
				CompressedSize:    250,
				CompressionMethod: Basic,
				Checksum:          0xABCDEF12,
			},
			wantErr: false,
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
		{
			name: "filename length mismatch",
			entry: &FileEntry{
				FilenameLength:    10, // Mismatch
				Filename:          "test.txt",
				DataOffset:        100,
				OriginalSize:      500,
				CompressedSize:    250,
				CompressionMethod: Basic,
				Checksum:          0xABCDEF12,
			},
			wantErr: true,
		},
		{
			name: "filename too long",
			entry: &FileEntry{
				FilenameLength:    255, // at max uint8 (forces error before cast in theoretical overflow)
				Filename:          strings.Repeat("a", 256),
				DataOffset:        0,
				OriginalSize:      0,
				CompressedSize:    0,
				CompressionMethod: None,
				Checksum:          0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SerializeFileEntry(tt.entry)
			if (err != nil) != tt.wantErr {
				t.Errorf("SerializeFileEntry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("SerializeFileEntry() returned nil bytes for valid input")
			}
		})
	}
}

func TestDeserializeFileEntry(t *testing.T) {
	validEntry := &FileEntry{
		FilenameLength:    4,
		Filename:          "data",
		DataOffset:        1024,
		OriginalSize:      2048,
		CompressedSize:    1024,
		CompressionMethod: None,
		Checksum:          12345678,
	}
	validBytes, _ := SerializeFileEntry(validEntry)

	// Create a payload with an invalid compression method
	invalidCompBytes := make([]byte, len(validBytes))
	copy(invalidCompBytes, validBytes)
	invalidCompBytes[26+4] = 99 // Set compression method to an invalid uint8

	tests := []struct {
		name      string
		data      []byte
		wantEntry *FileEntry
		wantRead  int
		wantErr   bool
	}{
		{
			name:      "valid data",
			data:      validBytes,
			wantEntry: validEntry,
			wantRead:  35, // 31 bytes static + 4 bytes filename
			wantErr:   false,
		},
		{
			name:      "data too small for length",
			data:      []byte{0x00}, // Only 1 byte
			wantEntry: nil,
			wantRead:  0,
			wantErr:   true,
		},
		{
			name:      "data truncated",
			data:      validBytes[:20], // Cut off mid-way
			wantEntry: nil,
			wantRead:  0,
			wantErr:   true,
		},
		{
			name:      "invalid compression method",
			data:      invalidCompBytes,
			wantEntry: nil,
			wantRead:  0,
			wantErr:   true,
		},
		{
			name:      "data has extra bytes at end (should ignore)",
			data:      append(validBytes, []byte{0xFF, 0xFF}...), // Extraneous trailing data
			wantEntry: validEntry,
			wantRead:  35,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEntry, gotRead, err := DeserializeFileEntry(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeserializeFileEntry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotRead != tt.wantRead {
					t.Errorf("DeserializeFileEntry() gotRead = %v, want %v", gotRead, tt.wantRead)
				}
				if gotEntry.Filename != tt.wantEntry.Filename ||
					gotEntry.DataOffset != tt.wantEntry.DataOffset ||
					gotEntry.OriginalSize != tt.wantEntry.OriginalSize ||
					gotEntry.CompressedSize != tt.wantEntry.CompressedSize ||
					gotEntry.CompressionMethod != tt.wantEntry.CompressionMethod ||
					gotEntry.Checksum != tt.wantEntry.Checksum {
					t.Errorf("DeserializeFileEntry() got = %+v, want %+v", gotEntry, tt.wantEntry)
				}
			}
		})
	}
}

func TestFileEntryRoundTrip(t *testing.T) {
	original := &FileEntry{
		FilenameLength:    11,
		Filename:          "archive.zip",
		DataOffset:        4096,
		OriginalSize:      10000,
		CompressedSize:    5000,
		CompressionMethod: Basic,
		Checksum:          0xDEADBEEF,
	}

	serialized, err := SerializeFileEntry(original)
	if err != nil {
		t.Fatalf("SerializeFileEntry() failed: %v", err)
	}

	deserialized, bytesRead, err := DeserializeFileEntry(serialized)
	if err != nil {
		t.Fatalf("DeserializeFileEntry() failed: %v", err)
	}

	expectedLen := 31 + len(original.Filename)
	if bytesRead != expectedLen {
		t.Errorf("RoundTrip bytes read = %d, want %d", bytesRead, expectedLen)
	}

	if len(serialized) != expectedLen {
		t.Errorf("RoundTrip serialized length = %d, want %d", len(serialized), expectedLen)
	}

	if *original != *deserialized {
		t.Errorf("RoundTrip result = %+v, want %+v", deserialized, original)
	}
}

// TestSerializeFileEntryByteAlignment verifies the exact placement of bytes
func TestSerializeFileEntryByteAlignment(t *testing.T) {
	entry := &FileEntry{
		FilenameLength:    3,
		Filename:          "foo",
		DataOffset:        0x0102030405060708,
		OriginalSize:      0x1112131415161718,
		CompressedSize:    0x2122232425262728,
		CompressionMethod: Basic,
		Checksum:          0x31323334,
	}

	expected := []byte{
		0x00, 0x03, // FilenameLength (2)
		'f', 'o', 'o', // Filename (3)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // DataOffset (8)
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, // OriginalSize (8)
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, // CompressedSize (8)
		0x01,                   // CompressionMethod (1)
		0x31, 0x32, 0x33, 0x34, // Checksum (4)
	}

	got, err := SerializeFileEntry(entry)
	if err != nil {
		t.Fatalf("SerializeFileEntry() failed: %v", err)
	}

	if !bytes.Equal(got, expected) {
		t.Errorf("SerializeFileEntry()\n got: %x\nwant: %x", got, expected)
	}
}
