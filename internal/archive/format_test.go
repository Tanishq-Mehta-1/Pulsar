package archive

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func validEntry(name string) FileEntry {
	return FileEntry{
		FilenameLength:    uint8(len(name)),
		Filename:          name,
		DataOffset:        12,
		OriginalSize:      100,
		CompressedSize:    100,
		CompressionMethod: None,
		Checksum:          0xCAFEBABE,
	}
}

// roundTripEntry serializes and deserializes an entry, capturing panics.
func roundTripEntry(e FileEntry) (got *FileEntry, consumed int, serializedLen int, err error, panicked any) {
	panicked = capturePanic(func() {
		var buf []byte
		buf, err = SerializeFileEntry(&e)
		if err != nil {
			return
		}
		serializedLen = len(buf)
		got, consumed, err = DeserializeFileEntry(buf)
	})
	return
}

func TestFileEntryConstants(t *testing.T) {
	// 1 (name length) + 8 + 8 + 8 + 1 (method) + 4 (checksum)
	if FileEntryLength != 30 {
		t.Fatalf("FileEntryLength = %d, want 30", FileEntryLength)
	}
}

func TestSerializeFileEntry_Layout(t *testing.T) {
	e := FileEntry{
		FilenameLength:    3,
		Filename:          "abc",
		DataOffset:        0x0102030405060708,
		OriginalSize:      0x1112131415161718,
		CompressedSize:    0x2122232425262728,
		CompressionMethod: BestSpeed,
		Checksum:          0xDEADBEEF,
	}
	want := []byte{
		3,
		'a', 'b', 'c',
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		1,
		0xDE, 0xAD, 0xBE, 0xEF,
	}
	got, err := SerializeFileEntry(&e)
	if err != nil {
		t.Fatalf("SerializeFileEntry: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got  % x\nwant % x", got, want)
	}
}

func TestSerializeFileEntry_Length(t *testing.T) {
	for _, n := range []int{0, 1, 2, 10, 128, 254, 255} {
		t.Run(fmt.Sprintf("name_len_%d", n), func(t *testing.T) {
			e := validEntry(strings.Repeat("x", n))
			buf, err := SerializeFileEntry(&e)
			if err != nil {
				t.Fatalf("SerializeFileEntry: %v", err)
			}
			if len(buf) != FileEntryLength+n {
				t.Fatalf("len = %d, want %d", len(buf), FileEntryLength+n)
			}
			if int(buf[0]) != n {
				t.Fatalf("length prefix = %d, want %d", buf[0], n)
			}
		})
	}
}

func TestSerializeFileEntry_CompressionMethodByte(t *testing.T) {
	for _, m := range []CompressionMethod{None, BestSpeed, BestCompression, Default, HuffmanOnly} {
		e := validEntry("f")
		e.CompressionMethod = m
		buf, err := SerializeFileEntry(&e)
		if err != nil {
			t.Fatalf("method %d: %v", m, err)
		}
		if got := buf[25+1]; got != byte(m) {
			t.Fatalf("method byte = %d, want %d", got, m)
		}
	}
}

func TestSerializeFileEntry_Rejects(t *testing.T) {
	tooLong := strings.Repeat("x", 256)
	tests := []struct {
		name  string
		entry *FileEntry
	}{
		{"nil entry", nil},
		{"filename 256 bytes, prefix 255", &FileEntry{FilenameLength: 255, Filename: tooLong}},
		{"filename 256 bytes, prefix 0 (wrapped)", &FileEntry{FilenameLength: 0, Filename: tooLong}},
		{"length prefix smaller than name", &FileEntry{FilenameLength: 2, Filename: "abc"}},
		{"length prefix larger than name", &FileEntry{FilenameLength: 4, Filename: "abc"}},
		{"nonzero prefix, empty name", &FileEntry{FilenameLength: 1, Filename: ""}},
		{"zero prefix, non-empty name", &FileEntry{FilenameLength: 0, Filename: "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf []byte
			var err error
			if p := capturePanic(func() { buf, err = SerializeFileEntry(tc.entry) }); p != nil {
				t.Fatalf("panic: %v", p)
			}
			if err == nil {
				t.Fatalf("expected error, got %d bytes", len(buf))
			}
			if buf != nil {
				t.Fatalf("expected nil buffer with error")
			}
		})
	}
}

func TestFileEntryRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		entry FileEntry
	}{
		{"empty filename", validEntry("")},
		{"one byte filename", validEntry("a")},
		{"typical", validEntry("hello.txt")},
		{"multibyte utf8 name", validEntry("файл-日本.bin")},
		{"name with spaces and dots", validEntry("my file.tar.gz")},
		{"name with NUL byte", validEntry("a\x00b")},
		{"229 byte name", validEntry(strings.Repeat("n", 229))},
		{
			"max numeric fields",
			FileEntry{
				FilenameLength: 1, Filename: "m",
				DataOffset: math.MaxUint64, OriginalSize: math.MaxUint64, CompressedSize: math.MaxUint64,
				CompressionMethod: BestSpeed, Checksum: math.MaxUint32,
			},
		},
		{
			"zero numeric fields",
			FileEntry{FilenameLength: 1, Filename: "z", CompressionMethod: None},
		},
		{
			"compressed smaller than original",
			FileEntry{FilenameLength: 1, Filename: "c", DataOffset: 12, OriginalSize: 1000, CompressedSize: 400, CompressionMethod: BestSpeed, Checksum: 7},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, consumed, serLen, err, p := roundTripEntry(tc.entry)
			if p != nil {
				t.Fatalf("panic: %v", p)
			}
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if *got != tc.entry {
				t.Fatalf("got %+v, want %+v", *got, tc.entry)
			}
			if consumed != serLen || consumed != FileEntryLength+len(tc.entry.Filename) {
				t.Fatalf("consumed %d, serialized %d, want %d", consumed, serLen, FileEntryLength+len(tc.entry.Filename))
			}
		})
	}
}

// The format allows filename lengths 0..255 (uint8 prefix, enforced by the
// serializer and the writer). Every legal length must survive a round trip.
func TestFileEntryRoundTrip_AllLegalFilenameLengths(t *testing.T) {
	var failing []int
	var firstProblem string
	for n := 0; n <= 255; n++ {
		e := validEntry(strings.Repeat("n", n))
		got, consumed, _, err, p := roundTripEntry(e)
		var problem string
		switch {
		case p != nil:
			problem = fmt.Sprintf("panic: %v", p)
		case err != nil:
			problem = fmt.Sprintf("error: %v", err)
		case *got != e || consumed != FileEntryLength+n:
			problem = "decoded entry differs from original"
		}
		if problem != "" {
			if len(failing) == 0 {
				firstProblem = problem
			}
			failing = append(failing, n)
		}
	}
	if len(failing) > 0 {
		t.Errorf("round trip broken for %d filename lengths (first %d, last %d); first failure: %s",
			len(failing), failing[0], failing[len(failing)-1], firstProblem)
	}
}

func TestFileEntryRoundTrip_FilenameBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 229, 230, 254, 255} {
		t.Run(fmt.Sprintf("name_len_%d", n), func(t *testing.T) {
			e := validEntry(strings.Repeat("b", n))
			got, _, _, err, p := roundTripEntry(e)
			if p != nil {
				t.Fatalf("DeserializeFileEntry panicked: %v", p)
			}
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if *got != e {
				t.Fatalf("decoded entry differs from original")
			}
		})
	}
}

func TestDeserializeFileEntry_Truncated(t *testing.T) {
	good, err := SerializeFileEntry(func() *FileEntry { e := validEntry("hello"); return &e }())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("every strict prefix of a valid entry", func(t *testing.T) {
		for i := 0; i < len(good); i++ {
			var gotErr error
			if p := capturePanic(func() { _, _, gotErr = DeserializeFileEntry(good[:i]) }); p != nil {
				t.Fatalf("prefix %d: panic: %v", i, p)
			}
			if gotErr == nil {
				t.Fatalf("prefix of %d/%d bytes accepted", i, len(good))
			}
		}
	})

	tests := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"one byte", []byte{0}},
		{"29 bytes (fixed part minus one)", make([]byte, FileEntryLength-1)},
		{"fixed part only, prefix claims 10-byte name", append([]byte{10}, make([]byte, FileEntryLength-1)...)},
		{"prefix claims 255, only 30 bytes", append([]byte{255}, make([]byte, FileEntryLength-1)...)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e *FileEntry
			var n int
			var err error
			if p := capturePanic(func() { e, n, err = DeserializeFileEntry(tc.data) }); p != nil {
				t.Fatalf("panic: %v", p)
			}
			if err == nil {
				t.Fatalf("expected error, got %+v", e)
			}
			if e != nil || n != 0 {
				t.Fatalf("expected (nil, 0) with error, got (%v, %d)", e, n)
			}
		})
	}
}

func TestDeserializeFileEntry_CompressionMethod(t *testing.T) {
	for _, name := range []string{"", "ab"} {
		base := validEntry(name)
		buf, err := SerializeFileEntry(&base)
		if err != nil {
			t.Fatal(err)
		}
		methodIdx := 1 + len(name) + 24

		for m := 0; m <= 255; m++ {
			data := append([]byte{}, buf...)
			data[methodIdx] = byte(m)
			var got *FileEntry
			var derr error
			if p := capturePanic(func() { got, _, derr = DeserializeFileEntry(data) }); p != nil {
				t.Fatalf("name %q method %d: panic %v", name, m, p)
			}
			valid := false
			switch CompressionMethod(m) {
			case None, BestSpeed, BestCompression, Default, HuffmanOnly:
				valid = true
			}
			if valid && derr != nil {
				t.Errorf("name %q method %d rejected: %v", name, m, derr)
			}
			if !valid && derr == nil {
				t.Errorf("name %q invalid method %d accepted: %+v", name, m, got)
			}
		}
	}
}

func TestDeserializeFileEntry_MultipleEntries(t *testing.T) {
	names := []string{"", "a", "second.bin", strings.Repeat("q", 100), "last"}
	var entries []FileEntry
	var dir []byte
	for i, n := range names {
		e := validEntry(n)
		e.DataOffset = uint64(12 + i*1000)
		e.OriginalSize = uint64(i)
		e.CompressedSize = uint64(i)
		e.Checksum = uint32(0x01010101 * (i + 1))
		entries = append(entries, e)
		b, err := SerializeFileEntry(&e)
		if err != nil {
			t.Fatal(err)
		}
		dir = append(dir, b...)
	}

	t.Run("sequential decode with returned counts", func(t *testing.T) {
		pos := 0
		for i, want := range entries {
			got, n, err := DeserializeFileEntry(dir[pos:])
			if err != nil {
				t.Fatalf("entry %d at %d: %v", i, pos, err)
			}
			if *got != want {
				t.Fatalf("entry %d: got %+v, want %+v", i, *got, want)
			}
			if n != FileEntryLength+len(want.Filename) {
				t.Fatalf("entry %d: consumed %d, want %d", i, n, FileEntryLength+len(want.Filename))
			}
			pos += n
		}
		if pos != len(dir) {
			t.Fatalf("consumed %d of %d bytes", pos, len(dir))
		}
	})

	t.Run("decoding one entry leaves the next untouched", func(t *testing.T) {
		got, n, err := DeserializeFileEntry(dir)
		if err != nil {
			t.Fatal(err)
		}
		if *got != entries[0] {
			t.Fatalf("first entry decoded wrongly: %+v", *got)
		}
		if n != FileEntryLength+len(entries[0].Filename) {
			t.Fatalf("consumed %d bytes, entry is %d", n, FileEntryLength+len(entries[0].Filename))
		}
		next, _, err := DeserializeFileEntry(dir[n:])
		if err != nil || *next != entries[1] {
			t.Fatalf("second entry not intact after first decode: %+v, %v", next, err)
		}
	})

	t.Run("trailing garbage after entry is not consumed", func(t *testing.T) {
		one, _ := SerializeFileEntry(&entries[2])
		data := append(append([]byte{}, one...), 0xFF, 0xFF, 0xFF)
		got, n, err := DeserializeFileEntry(data)
		if err != nil {
			t.Fatal(err)
		}
		if *got != entries[2] || n != len(one) {
			t.Fatalf("got %+v consumed %d, want %+v consumed %d", *got, n, entries[2], len(one))
		}
	})
}

// Malformed bytes must produce an error or a decoded entry, never a panic.
func TestDeserializeFileEntry_GarbageDoesNotPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var panicking []int
	var first any
	for n := 0; n <= 255; n++ {
		for trial := 0; trial < 4; trial++ {
			size := FileEntryLength + n
			if trial%2 == 1 {
				size += rng.Intn(40) // extra trailing bytes
			}
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = byte(rng.Intn(256))
			}
			buf[0] = byte(n)
			buf[1+n+24] = byte(trial % 2) // plausible compression method

			if p := capturePanic(func() { _, _, _ = DeserializeFileEntry(buf) }); p != nil {
				if len(panicking) == 0 || panicking[len(panicking)-1] != n {
					panicking = append(panicking, n)
				}
				if first == nil {
					first = p
				}
			}
		}
	}
	if len(panicking) > 0 {
		t.Errorf("DeserializeFileEntry panicked for %d name-length values (first %d, last %d); first panic: %v",
			len(panicking), panicking[0], panicking[len(panicking)-1], first)
	}
}
