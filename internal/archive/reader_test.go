package archive

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func baseFiles() []blob {
	// The last data byte is 'x' (0x78 = 120) on purpose: misaligned directory
	// parses that start on it claim a 120-byte name and must be rejected.
	return []blob{
		{"a.txt", []byte("first file contents")},
		{"b.bin", []byte("second file contents x")},
	}
}

func extractTo(t *testing.T, r *ArchiveReader, name string) ([]byte, error) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "out")
	if err := r.ExtractFile(name, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			t.Errorf("ExtractFile(%q) failed but still created %s", name, dst)
		}
		return nil, err
	}
	return readTestFile(t, dst), nil
}

func TestOpenArchive_Valid(t *testing.T) {
	chdirTemp(t)
	files := baseFiles()
	path := buildWithWriter(t, "valid", files)
	r := openReader(t, path)

	if r.Status != Active {
		t.Fatalf("Status = %d, want Active", r.Status)
	}
	if len(r.Files) != len(files) {
		t.Fatalf("Files = %d, want %d", len(r.Files), len(files))
	}
	_, want, _ := parseArchiveBytes(t, readTestFile(t, path))
	for i := range want {
		if r.Files[i] != want[i] {
			t.Errorf("Files[%d] = %+v, want %+v", i, r.Files[i], want[i])
		}
	}
}

func TestOpenArchive_HandBuiltArchive(t *testing.T) {
	arch, _ := layoutArchive(t, baseFiles())
	r := openReader(t, writeArchiveBytes(t, arch))
	got, err := extractTo(t, r, "b.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second file contents x" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenArchive_EmptyArchive(t *testing.T) {
	arch := assembleArchive(t, nil, nil)
	r := openReader(t, writeArchiveBytes(t, arch))
	if r.Status != Active || len(r.Files) != 0 {
		t.Fatalf("status=%d files=%d", r.Status, len(r.Files))
	}
	if _, err := extractTo(t, r, "anything"); err == nil {
		t.Fatal("extracting from an empty archive should fail")
	}
}

func TestOpenArchive_NonexistentPath(t *testing.T) {
	r := &ArchiveReader{}
	if err := r.OpenArchive(filepath.Join(t.TempDir(), "nope.fz")); err == nil {
		t.Fatal("expected error")
	}
	if r.Status == Active {
		t.Fatal("reader Active after failed open")
	}
}

func TestOpenArchive_TooSmall(t *testing.T) {
	valid := assembleArchive(t, nil, nil) // 36 bytes: smallest legal archive
	if len(valid) != HeaderSize+FooterSize {
		t.Fatalf("setup: %d bytes", len(valid))
	}
	for _, size := range []int{0, 1, HeaderSize - 1, HeaderSize, FooterSize, HeaderSize + FooterSize - 1} {
		t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
			path := writeArchiveBytes(t, valid[:size])
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)
			if p := capturePanic(func() {
				if err := r.OpenArchive(path); err == nil {
					t.Error("expected error")
				}
			}); p != nil {
				t.Fatalf("panic: %v", p)
			}
			if r.Status == Active {
				t.Fatal("reader Active after failed open")
			}
		})
	}
}

func TestOpenArchive_RejectsMalformed(t *testing.T) {
	base, entries := layoutArchive(t, baseFiles())
	total := uint64(len(base))
	firstEntryMethodIdx := 1 + len(entries[0].Filename) + 24

	tests := []struct {
		name   string
		mutate func(t *testing.T, a []byte) []byte
	}{
		{"bad header signature", func(t *testing.T, a []byte) []byte { a[0] ^= 0xFF; return a }},
		{"unsupported version", func(t *testing.T, a []byte) []byte { a[5] = 2; return a }},
		{"nonzero flags", func(t *testing.T, a []byte) []byte { a[7] = 1; return a }},
		{"nonzero reserved", func(t *testing.T, a []byte) []byte { a[11] = 1; return a }},
		{"bad footer signature", func(t *testing.T, a []byte) []byte { a[len(a)-FooterSize] ^= 0xFF; return a }},
		{"directory offset +1", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectoryOffset++ })
		}},
		{"directory size +1", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectorySize++ })
		}},
		{"directory size -1", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectorySize-- })
		}},
		{"directory shifted one byte earlier (sum preserved)", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectoryOffset--; f.DirectorySize++ })
		}},
		{"directory covers whole file body", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectoryOffset = 0; f.DirectorySize = total - uint64(FooterSize) })
		}},
		{"directory offset beyond EOF", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.DirectoryOffset = total; f.DirectorySize = 0 })
		}},
		{"entry count too high", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.EntryCount++ })
		}},
		{"entry count too low", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.EntryCount-- })
		}},
		{"entry count zero with entries present", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) { f.EntryCount = 0 })
		}},
		{"directory truncated by 5 bytes (footer consistent)", func(t *testing.T, a []byte) []byte {
			d := directoryOf(t, a)
			return replaceDirectory(t, a, d[:len(d)-5], 2)
		}},
		{"directory truncated mid fixed fields", func(t *testing.T, a []byte) []byte {
			d := directoryOf(t, a)
			return replaceDirectory(t, a, d[:len(entries[0].Filename)+1+10], 2)
		}},
		{"trailing garbage after directory", func(t *testing.T, a []byte) []byte {
			return replaceDirectory(t, a, append(directoryOf(t, a), 1, 2, 3), 2)
		}},
		{"directory is all 0xFF", func(t *testing.T, a []byte) []byte {
			return replaceDirectory(t, a, bytes.Repeat([]byte{0xFF}, 70), 2)
		}},
		{"invalid compression method in directory", func(t *testing.T, a []byte) []byte {
			d := directoryOf(t, a)
			d[firstEntryMethodIdx] = 5 // not a defined method
			return replaceDirectory(t, a, d, 2)
		}},
		{"directory offset/size wrap around uint64", func(t *testing.T, a []byte) []byte {
			return patchFooter(t, a, func(f *Footer) {
				f.DirectoryOffset = 1 << 63
				f.DirectorySize = total - uint64(FooterSize) + 1<<63
			})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			arch := tc.mutate(t, append([]byte{}, base...))
			path := writeArchiveBytes(t, arch)
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)

			var err error
			if p := capturePanic(func() { err = r.OpenArchive(path) }); p != nil {
				t.Fatalf("OpenArchive panicked: %v", p)
			}
			if err == nil {
				t.Fatal("expected error for malformed archive")
			}
			if r.Status == Active {
				t.Error("reader is Active after a failed open")
			}
			if len(r.Files) != 0 {
				t.Errorf("reader kept %d entries after failed open", len(r.Files))
			}
			if err := r.ExtractFile("a.txt", filepath.Join(t.TempDir(), "o")); err == nil {
				t.Error("ExtractFile succeeded on a reader whose open failed")
			}
		})
	}
}

func TestOpenArchive_FailedOpenDoesNotLeakFileHandles(t *testing.T) {
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skip("/proc/self/fd not available on this platform")
	}
	countFDs := func() int {
		e, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(e)
	}

	base, _ := layoutArchive(t, baseFiles())
	cases := []struct {
		name string
		data []byte
	}{
		{"too small", base[:10]},
		{"bad header", func() []byte { a := append([]byte{}, base...); a[0] ^= 0xFF; return a }()},
		{"bad footer signature", func() []byte { a := append([]byte{}, base...); a[len(a)-FooterSize] ^= 0xFF; return a }()},
		{"inconsistent footer", patchFooter(t, base, func(f *Footer) { f.DirectorySize++ })},
		{"bad entry count", patchFooter(t, base, func(f *Footer) { f.EntryCount = 99 })},
		{"truncated directory", replaceDirectory(t, base, directoryOf(t, base)[:20], 2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeArchiveBytes(t, tc.data)
			runtime.GC() // flush finalizers from earlier tests so the baseline is stable
			// A leaked *os.File is closed by its finalizer once collected, which
			// would make the count vary; keep the GC off while measuring.
			defer debug.SetGCPercent(debug.SetGCPercent(-1))
			before := countFDs()
			const attempts = 5
			readers := make([]*ArchiveReader, attempts)
			for i := range readers {
				readers[i] = &ArchiveReader{}
				if err := readers[i].OpenArchive(path); err == nil {
					t.Fatal("expected open to fail")
				}
			}
			after := countFDs()
			// Close handles ourselves so the test does not leak into others.
			for _, r := range readers {
				if r.OutputFile != nil {
					_ = r.OutputFile.Close()
				}
			}
			if after > before {
				t.Errorf("%d file handle(s) still open after %d failed OpenArchive calls", after-before, attempts)
			}
		})
	}
}

func TestOpenArchive_FailedOpenCanBeRetried(t *testing.T) {
	good, _ := layoutArchive(t, baseFiles())
	bad := append([]byte{}, good...)
	bad[0] ^= 0xFF

	r := &ArchiveReader{}
	closeReaderOnCleanup(t, r)
	if err := r.OpenArchive(writeArchiveBytes(t, bad)); err == nil {
		t.Fatal("expected failure")
	}
	if err := r.OpenArchive(writeArchiveBytes(t, good)); err != nil {
		t.Fatalf("retry on the same reader failed: %v", err)
	}
	if r.Status != Active || len(r.Files) != 2 {
		t.Fatalf("status=%d files=%d", r.Status, len(r.Files))
	}
}

func TestReader_InvalidStates(t *testing.T) {
	arch, _ := layoutArchive(t, baseFiles())

	t.Run("ExtractFile before OpenArchive", func(t *testing.T) {
		r := &ArchiveReader{}
		dst := filepath.Join(t.TempDir(), "o")
		if p := capturePanic(func() {
			if err := r.ExtractFile("a.txt", dst); err == nil {
				t.Error("expected error")
			}
		}); p != nil {
			t.Fatalf("panic: %v", p)
		}
		if _, err := os.Stat(dst); err == nil {
			t.Error("destination created")
		}
	})

	t.Run("OpenArchive twice", func(t *testing.T) {
		path := writeArchiveBytes(t, arch)
		r := openReader(t, path)
		if err := r.OpenArchive(path); err == nil {
			t.Fatal("second OpenArchive should fail")
		}
		// the first open must still be usable
		got, err := extractTo(t, r, "a.txt")
		if err != nil || string(got) != "first file contents" {
			t.Fatalf("extract after rejected re-open: %q, %v", got, err)
		}
		if len(r.Files) != 2 {
			t.Fatalf("Files changed by rejected re-open: %d", len(r.Files))
		}
	})
}

func TestExtractFile_ContentShapes(t *testing.T) {
	chdirTemp(t)
	blobs := sampleBlobs()
	path := buildWithWriter(t, "shapes", blobs)
	r := openReader(t, path)

	for _, b := range blobs {
		t.Run(b.name, func(t *testing.T) {
			got, err := extractTo(t, r, b.name)
			if err != nil {
				t.Fatalf("ExtractFile: %v", err)
			}
			if !bytes.Equal(got, b.data) {
				t.Fatalf("extracted %d bytes differ from original %d bytes", len(got), len(b.data))
			}
		})
	}
}

func TestExtractFile_FilesAreIndependent(t *testing.T) {
	files := []blob{
		{"one", []byte("1111")},
		{"two", []byte("22")},
		{"three", []byte{0, 0, 0, 3}},
	}
	arch, _ := layoutArchive(t, files)
	r := openReader(t, writeArchiveBytes(t, arch))

	// reverse order, then each again: extraction must not depend on order or
	// on earlier extractions
	order := []int{2, 0, 1, 1, 0, 2}
	for _, i := range order {
		got, err := extractTo(t, r, files[i].name)
		if err != nil {
			t.Fatalf("%s: %v", files[i].name, err)
		}
		if !bytes.Equal(got, files[i].data) {
			t.Fatalf("%s: got %q, want %q", files[i].name, got, files[i].data)
		}
	}
}

func TestExtractFile_OverwritesExistingDestination(t *testing.T) {
	arch, _ := layoutArchive(t, baseFiles())
	r := openReader(t, writeArchiveBytes(t, arch))
	dst := writeTestFile(t, t.TempDir(), "out", bytes.Repeat([]byte("old"), 100))
	if err := r.ExtractFile("a.txt", dst); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, dst); string(got) != "first file contents" {
		t.Fatalf("destination = %q", got)
	}
}

func TestExtractFile_DestinationDirectoryMissing(t *testing.T) {
	arch, _ := layoutArchive(t, baseFiles())
	r := openReader(t, writeArchiveBytes(t, arch))
	dst := filepath.Join(t.TempDir(), "missing-dir", "out")
	if err := r.ExtractFile("a.txt", dst); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractFile_UnknownNames(t *testing.T) {
	arch, _ := layoutArchive(t, baseFiles())
	r := openReader(t, writeArchiveBytes(t, arch))
	names := map[string]string{
		"missing":          "missing.txt",
		"case differs":     "A.TXT",
		"prefix of real":   "a.tx",
		"trailing space":   "a.txt ",
		"with path":        "dir/a.txt",
		"longer than 255":  strings.Repeat("z", 256),
		"empty name":       "",
		"NUL byte":         "a.txt\x00",
		"another file ext": "a.bin",
	}
	for label, name := range names {
		t.Run(label, func(t *testing.T) {
			var err error
			var got []byte
			if p := capturePanic(func() { got, err = extractTo(t, r, name) }); p != nil {
				t.Fatalf("panic: %v", p)
			}
			if err == nil {
				t.Fatalf("ExtractFile(%q) succeeded and produced %d bytes; archive has no such file", name, len(got))
			}
		})
	}
}

func TestExtractFile_DetectsCorruptData(t *testing.T) {
	chdirTemp(t)
	path := buildWithWriter(t, "corrupt", baseFiles())
	orig := readTestFile(t, path)
	_, entries, _ := parseArchiveBytes(t, orig)

	t.Run("flipped byte in file data", func(t *testing.T) {
		b := append([]byte{}, orig...)
		b[entries[0].DataOffset+3] ^= 0x01
		r := openReader(t, writeArchiveBytes(t, b))
		if _, err := extractTo(t, r, "a.txt"); err == nil {
			t.Fatal("corrupted data extracted without error")
		}
		// neighbouring file is unaffected
		got, err := extractTo(t, r, "b.bin")
		if err != nil || string(got) != "second file contents x" {
			t.Fatalf("untouched file: %q, %v", got, err)
		}
	})

	t.Run("flipped byte in stored checksum", func(t *testing.T) {
		f := footerOf(t, orig)
		b := append([]byte{}, orig...)
		// last byte of the first directory entry is the low byte of its checksum
		b[f.DirectoryOffset+uint64(FileEntryLength+len(entries[0].Filename))-1] ^= 0x80
		r := openReader(t, writeArchiveBytes(t, b))
		if _, err := extractTo(t, r, "a.txt"); err == nil {
			t.Fatal("checksum mismatch not detected")
		}
	})

	t.Run("truncated data region", func(t *testing.T) {
		// directory claims more data than the file holds
		arch, ents := layoutArchive(t, baseFiles())
		ents[1].CompressedSize += 1000
		ents[1].OriginalSize += 1000
		bad := assembleArchive(t, arch[HeaderSize:footerOf(t, arch).DirectoryOffset], ents)
		r := &ArchiveReader{}
		closeReaderOnCleanup(t, r)
		if err := r.OpenArchive(writeArchiveBytes(t, bad)); err != nil {
			return // rejected at open time: acceptable
		}
		if _, err := extractTo(t, r, "b.bin"); err == nil {
			t.Fatal("expected error when data extends beyond end of archive")
		}
	})

	t.Run("checksum of every single-bit flip is caught", func(t *testing.T) {
		data := []byte("bit flip sweep")
		arch, _ := layoutArchive(t, []blob{{"f", data}})
		for bit := 0; bit < len(data)*8; bit++ {
			b := append([]byte{}, arch...)
			b[HeaderSize+bit/8] ^= 1 << (bit % 8)
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)
			if err := r.OpenArchive(writeArchiveBytes(t, b)); err != nil {
				t.Fatal(err)
			}
			if _, err := extractTo(t, r, "f"); err == nil {
				t.Fatalf("bit %d flip not detected", bit)
			}
		}
	})
}

func TestWriterReaderRoundTrip_FilenameLengths(t *testing.T) {
	for _, n := range []int{1, 100, 229, 230, 254, 255} {
		t.Run(fmt.Sprintf("name_len_%d", n), func(t *testing.T) {
			dir := chdirTemp(t)
			name := strings.Repeat("r", n)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				t.Skipf("filesystem rejects %d-byte names: %v", n, err)
			}
			content := []byte("round trip contents")
			var path string
			if p := capturePanic(func() { path = buildWithWriter(t, "rt", []blob{{name, content}}) }); p != nil {
				t.Fatalf("writer panicked: %v", p)
			}

			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)
			var openErr error
			if p := capturePanic(func() { openErr = r.OpenArchive(path) }); p != nil {
				t.Fatalf("OpenArchive panicked on an archive produced by ArchiveWriter: %v", p)
			}
			if openErr != nil {
				t.Fatalf("OpenArchive: %v", openErr)
			}
			got, err := extractTo(t, r, name)
			if err != nil {
				t.Fatalf("ExtractFile: %v", err)
			}
			if !bytes.Equal(got, content) {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestExtractFile_DuplicateNamesReturnFirst(t *testing.T) {
	files := []blob{{"dup", []byte("FIRST")}, {"dup", []byte("SECOND!")}}
	arch, _ := layoutArchive(t, files)
	r := openReader(t, writeArchiveBytes(t, arch))
	got, err := extractTo(t, r, "dup")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "FIRST" {
		t.Fatalf("got %q", got)
	}
	t.Log("note: with duplicate names the second entry is unreachable through ExtractFile")
}

// ---- data-region validation ------------------------------------------------
//
// The reader trusts DataOffset/CompressedSize from the directory. These tests
// craft entries whose data region is not inside [HeaderSize, DirectoryOffset)
// but whose checksum is correct for the bytes found there. A strict reader
// would reject them at open time or at extraction time.

// archiveWithRegion builds an archive containing one legitimate file and an
// "evil" entry pointing at the region chosen by regionFn (computed against a
// first-pass layout; chosen regions never include the evil entry's own bytes).
func archiveWithRegion(t *testing.T, regionFn func(first []byte, dirOff uint64) (off, size uint64)) []byte {
	t.Helper()
	payload := []byte("legit payload")
	legit := entryForData("legit.txt", uint64(HeaderSize), payload)
	evil := FileEntry{FilenameLength: 4, Filename: "evil", CompressionMethod: None}

	pass1 := assembleArchive(t, payload, []FileEntry{legit, evil})
	off, size := regionFn(pass1, footerOf(t, pass1).DirectoryOffset)
	evil.DataOffset, evil.OriginalSize, evil.CompressedSize = off, size, size
	evil.Checksum = crc32.ChecksumIEEE(pass1[off : off+size])
	return assembleArchive(t, payload, []FileEntry{legit, evil})
}

func TestReader_RejectsDataRegionsOutsideDataSection(t *testing.T) {
	legitEntrySize := uint64(FileEntryLength + len("legit.txt"))
	tests := []struct {
		name   string
		region func(first []byte, dirOff uint64) (off, size uint64)
	}{
		{"region inside the header", func(_ []byte, _ uint64) (uint64, uint64) { return 0, uint64(HeaderSize) }},
		{"region overlapping header and data", func(_ []byte, _ uint64) (uint64, uint64) { return 6, 10 }},
		{"region inside the central directory", func(_ []byte, d uint64) (uint64, uint64) { return d, legitEntrySize }},
		{"region straddling data end and directory start", func(_ []byte, d uint64) (uint64, uint64) { return d - 5, 10 }},
		{"region equal to the footer", func(f []byte, _ uint64) (uint64, uint64) { return uint64(len(f) - FooterSize), uint64(FooterSize) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			arch := archiveWithRegion(t, tc.region)
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)
			if err := r.OpenArchive(writeArchiveBytes(t, arch)); err != nil {
				return // rejected at open time: acceptable
			}
			got, err := extractTo(t, r, "evil")
			if err == nil {
				t.Errorf("ExtractFile returned %d bytes taken from outside the data section (%q...)", len(got), truncate(got, 16))
			}
		})
	}
}

func TestReader_RejectsOversizedCompressedSizeWithoutPanic(t *testing.T) {
	legit := entryForData("legit.txt", uint64(HeaderSize), []byte("legit payload"))
	for _, size := range []uint64{1 << 62, 1<<63 + 1, ^uint64(0)} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			evil := FileEntry{
				FilenameLength: 4, Filename: "evil",
				DataOffset: uint64(HeaderSize), OriginalSize: size, CompressedSize: size, CompressionMethod: None,
			}
			arch := assembleArchive(t, []byte("legit payload"), []FileEntry{legit, evil})
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)

			var err error
			p := capturePanic(func() {
				if err = r.OpenArchive(writeArchiveBytes(t, arch)); err != nil {
					return
				}
				_, err = extractTo(t, r, "evil")
			})
			if p != nil {
				t.Fatalf("panicked on an entry claiming %d bytes: %v", size, p)
			}
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestReader_RejectsDataOffsetBeyondEOF(t *testing.T) {
	tests := []struct {
		name         string
		off, compLen uint64
	}{
		{"offset past EOF", 1 << 20, 4},
		{"region crosses EOF", 0, 0}, // filled below
		{"offset with high bit set", 1 << 63, 4},
		{"offset max uint64", ^uint64(0), 2},
	}
	base, _ := layoutArchive(t, baseFiles())
	tests[1].off = uint64(len(base)) - 2
	tests[1].compLen = 10

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := FileEntry{
				FilenameLength: 1, Filename: "e",
				DataOffset: tc.off, OriginalSize: tc.compLen, CompressedSize: tc.compLen, CompressionMethod: None,
			}
			arch := assembleArchive(t, []byte("payload"), []FileEntry{e})
			r := &ArchiveReader{}
			closeReaderOnCleanup(t, r)
			var err error
			if p := capturePanic(func() {
				if err = r.OpenArchive(writeArchiveBytes(t, arch)); err != nil {
					return
				}
				_, err = extractTo(t, r, "e")
			}); p != nil {
				t.Fatalf("panic: %v", p)
			}
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// For method None the stored bytes are the original bytes, so the two sizes
// must agree. The current reader never compares them.
func TestReader_RejectsUncompressedEntryWithMismatchedSizes(t *testing.T) {
	data := []byte("some stored data")
	e := entryForData("m", uint64(HeaderSize), data)
	e.OriginalSize = uint64(len(data)) + 5 // checksum still matches the stored bytes
	arch := assembleArchive(t, data, []FileEntry{e})

	r := &ArchiveReader{}
	closeReaderOnCleanup(t, r)
	if err := r.OpenArchive(writeArchiveBytes(t, arch)); err != nil {
		return // rejected at open time: acceptable
	}
	if _, err := extractTo(t, r, "m"); err == nil {
		t.Error("extraction succeeded although OriginalSize != CompressedSize for CompressionMethod None")
	}
}

func truncate(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

var allMethods = []struct {
	name   string
	method CompressionMethod
}{
	{"None", None},
	{"Default", Default},
	{"BestSpeed", BestSpeed},
	{"BestCompression", BestCompression},
	{"HuffmanOnly", HuffmanOnly},
}

func TestWriterReaderRoundTrip_AllMethods(t *testing.T) {
	for _, m := range allMethods {
		t.Run(m.name, func(t *testing.T) {
			chdirTemp(t)
			blobs := sampleBlobs()
			path := buildWithWriterMethod(t, "rtm", blobs, m.method)
			r := openReader(t, path)
			for _, b := range blobs {
				got, err := extractTo(t, r, b.name)
				if err != nil {
					t.Fatalf("%s: %v", b.name, err)
				}
				if !bytes.Equal(got, b.data) {
					t.Fatalf("%s: extracted %d bytes differ from original %d", b.name, len(got), len(b.data))
				}
			}
			for _, e := range r.Files {
				if e.CompressionMethod != m.method {
					t.Errorf("%s: method = %d, want %d", e.Filename, e.CompressionMethod, m.method)
				}
			}
		})
	}
}

func TestExtractFile_MixedMethodsInOneArchive(t *testing.T) {
	dir := chdirTemp(t)
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("mixed"); err != nil {
		t.Fatal(err)
	}
	var blobs []blob
	for i, m := range allMethods {
		b := blob{fmt.Sprintf("f%d.bin", i), bytes.Repeat([]byte("abc"), 1000+i)}
		blobs = append(blobs, b)
		if err := w.AddFile(writeTestFile(t, src, b.name, b.data), m.method); err != nil {
			t.Fatalf("AddFile %s: %v", m.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := openReader(t, filepath.Join(dir, "mixed.fz"))
	for _, b := range blobs {
		got, err := extractTo(t, r, b.name)
		if err != nil || !bytes.Equal(got, b.data) {
			t.Fatalf("%s: err=%v equal=%v", b.name, err, bytes.Equal(got, b.data))
		}
	}
}

func TestExtractFile_CompressedCorruption(t *testing.T) {
	chdirTemp(t)
	data := bytes.Repeat([]byte("compressible "), 500)
	path := buildWithWriterMethod(t, "cc", []blob{{"c.txt", data}}, BestCompression)
	orig := readTestFile(t, path)
	_, entries, _ := parseArchiveBytes(t, orig)
	e := entries[0]

	t.Run("flipped byte in compressed stream", func(t *testing.T) {
		b := append([]byte{}, orig...)
		b[e.DataOffset+e.CompressedSize/2] ^= 0xFF
		r := openReader(t, writeArchiveBytes(t, b))
		if _, err := extractTo(t, r, "c.txt"); err == nil {
			t.Fatal("corrupted compressed data extracted without error")
		}
	})

	t.Run("wrong original size", func(t *testing.T) {
		ents := append([]FileEntry{}, entries...)
		ents[0].OriginalSize += 7
		bad := assembleArchive(t, orig[HeaderSize:footerOf(t, orig).DirectoryOffset], ents)
		r := openReader(t, writeArchiveBytes(t, bad))
		if _, err := extractTo(t, r, "c.txt"); err == nil {
			t.Fatal("size mismatch not detected")
		}
	})

	t.Run("wrong checksum", func(t *testing.T) {
		ents := append([]FileEntry{}, entries...)
		ents[0].Checksum ^= 1
		bad := assembleArchive(t, orig[HeaderSize:footerOf(t, orig).DirectoryOffset], ents)
		r := openReader(t, writeArchiveBytes(t, bad))
		if _, err := extractTo(t, r, "c.txt"); err == nil {
			t.Fatal("checksum mismatch not detected")
		}
	})

	t.Run("garbage that is not a deflate stream", func(t *testing.T) {
		junk := bytes.Repeat([]byte{0xFF}, 64)
		ent := entryForData("j", uint64(HeaderSize), junk)
		ent.CompressionMethod = Default
		ent.OriginalSize = 10
		r := openReader(t, writeArchiveBytes(t, assembleArchive(t, junk, []FileEntry{ent})))
		if p := capturePanic(func() {
			if _, err := extractTo(t, r, "j"); err == nil {
				t.Error("expected error")
			}
		}); p != nil {
			t.Fatalf("panic: %v", p)
		}
	})
}
