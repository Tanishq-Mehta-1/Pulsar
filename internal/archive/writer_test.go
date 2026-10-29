package archive

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateArchive_WritesValidHeader(t *testing.T) {
	dir := chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)

	if err := w.CreateArchive("hdr"); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	if w.Status != Active {
		t.Fatalf("Status = %d, want Active", w.Status)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b := readTestFile(t, filepath.Join(dir, "hdr.fz"))
	want := []byte{0x11, 0x21, 0x31, 0x41, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(b[:HeaderSize], want) {
		t.Fatalf("header bytes % x, want % x", b[:HeaderSize], want)
	}
	if _, err := DeserializeHeader([HeaderSize]byte(b[:HeaderSize])); err != nil {
		t.Fatalf("written header does not validate: %v", err)
	}
}

func TestCreateArchive_FileNames(t *testing.T) {
	tests := []struct {
		in, wantFile string
	}{
		{"plain", "plain.fz"},
		{"", "output.fz"},
		{"with space", "with space.fz"},
		{"multi.dot.name", "multi.dot.name.fz"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.in), func(t *testing.T) {
			dir := chdirTemp(t)
			w := &ArchiveWriter{}
			closeWriterOnCleanup(t, w)
			if err := w.CreateArchive(tc.in); err != nil {
				t.Fatalf("CreateArchive: %v", err)
			}
			info, err := os.Stat(filepath.Join(dir, tc.wantFile))
			if err != nil {
				t.Fatalf("expected %s to exist: %v", tc.wantFile, err)
			}
			if info.Size() != int64(HeaderSize) {
				t.Fatalf("new archive size = %d, want %d (header only)", info.Size(), HeaderSize)
			}
		})
	}
}

func TestCreateArchive_RejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"a/b", "/abs", "dir/", strings.Repeat("a", 256)} {
		t.Run(fmt.Sprintf("len%d_%.8s", len(name), name), func(t *testing.T) {
			dir := chdirTemp(t)
			w := &ArchiveWriter{}
			closeWriterOnCleanup(t, w)
			if err := w.CreateArchive(name); err == nil {
				t.Fatal("expected error")
			}
			if w.Status != Created {
				t.Fatalf("Status = %d, want Created after failed create", w.Status)
			}
			if w.OutputFile != nil {
				t.Fatal("OutputFile set after failed create")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("failed create left %d file(s) behind", len(entries))
			}
		})
	}
}

func TestArchiveWriter_InvalidStates(t *testing.T) {
	t.Run("AddFile before CreateArchive", func(t *testing.T) {
		dir := chdirTemp(t)
		src := writeTestFile(t, dir, "x.txt", []byte("x"))
		w := &ArchiveWriter{}
		if err := w.AddFile(src); err == nil {
			t.Fatal("expected error")
		}
		if w.Status != Created || len(w.Files) != 0 {
			t.Fatalf("state changed: status=%d files=%d", w.Status, len(w.Files))
		}
	})

	t.Run("Close before CreateArchive", func(t *testing.T) {
		w := &ArchiveWriter{}
		if err := w.Close(); err == nil {
			t.Fatal("expected error")
		}
		if w.Status != Created {
			t.Fatalf("Status = %d", w.Status)
		}
	})

	t.Run("CreateArchive twice", func(t *testing.T) {
		dir := chdirTemp(t)
		w := &ArchiveWriter{}
		closeWriterOnCleanup(t, w)
		if err := w.CreateArchive("one"); err != nil {
			t.Fatal(err)
		}
		if err := w.CreateArchive("two"); err == nil {
			t.Fatal("second CreateArchive should fail")
		}
		if _, err := os.Stat(filepath.Join(dir, "two.fz")); err == nil {
			t.Fatal("second CreateArchive created a file")
		}
		if w.Status != Active {
			t.Fatalf("Status = %d, want Active", w.Status)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("writer unusable after rejected second create: %v", err)
		}
	})

	t.Run("Close twice", func(t *testing.T) {
		chdirTemp(t)
		w := &ArchiveWriter{}
		closeWriterOnCleanup(t, w)
		if err := w.CreateArchive("c"); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if w.Status != Closed {
			t.Fatalf("Status = %d, want Closed", w.Status)
		}
		if err := w.Close(); err == nil {
			t.Fatal("second Close should fail")
		}
		if w.Status != Closed {
			t.Fatalf("Status changed by failed Close: %d", w.Status)
		}
	})

	t.Run("AddFile and CreateArchive after Close", func(t *testing.T) {
		dir := chdirTemp(t)
		src := writeTestFile(t, dir, "x.txt", []byte("x"))
		w := &ArchiveWriter{}
		closeWriterOnCleanup(t, w)
		if err := w.CreateArchive("c"); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		before := readTestFile(t, filepath.Join(dir, "c.fz"))
		if err := w.AddFile(src); err == nil {
			t.Fatal("AddFile after Close should fail")
		}
		if err := w.CreateArchive("again"); err == nil {
			t.Fatal("CreateArchive after Close should fail")
		}
		if after := readTestFile(t, filepath.Join(dir, "c.fz")); !bytes.Equal(before, after) {
			t.Fatal("closed archive was modified")
		}
	})
}

func TestAddFile_SingleFileMetadataAndData(t *testing.T) {
	dir := chdirTemp(t)
	content := []byte("hello world\n")
	src := writeTestFile(t, dir, "greeting.txt", content)

	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("single"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(src); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if len(w.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(w.Files))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := readTestFile(t, filepath.Join(dir, "single.fz"))
	_, entries, _ := parseArchiveBytes(t, b)
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want 1", len(entries))
	}
	want := FileEntry{
		FilenameLength:    uint8(len("greeting.txt")),
		Filename:          "greeting.txt",
		DataOffset:        uint64(HeaderSize),
		OriginalSize:      uint64(len(content)),
		CompressedSize:    uint64(len(content)),
		CompressionMethod: None,
		Checksum:          crc32.ChecksumIEEE(content),
	}
	if entries[0] != want {
		t.Fatalf("entry = %+v\nwant   %+v", entries[0], want)
	}
	if w.Files[0] != want {
		t.Fatalf("in-memory entry = %+v, want %+v", w.Files[0], want)
	}
	got := b[entries[0].DataOffset : entries[0].DataOffset+entries[0].CompressedSize]
	if !bytes.Equal(got, content) {
		t.Fatalf("archived data = %q, want %q", got, content)
	}
}

func TestAddFile_ContentShapes(t *testing.T) {
	for i, f := range sampleBlobs() {
		t.Run(f.name, func(t *testing.T) {
			dir := chdirTemp(t)
			src := writeTestFile(t, dir, f.name, f.data)
			w := &ArchiveWriter{}
			closeWriterOnCleanup(t, w)
			name := fmt.Sprintf("shape%d", i)
			if err := w.CreateArchive(name); err != nil {
				t.Fatal(err)
			}
			if err := w.AddFile(src); err != nil {
				t.Fatalf("AddFile: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			b := readTestFile(t, filepath.Join(dir, name+".fz"))
			_, entries, ftr := parseArchiveBytes(t, b)
			if len(entries) != 1 {
				t.Fatalf("entries = %d", len(entries))
			}
			e := entries[0]
			if e.OriginalSize != uint64(len(f.data)) || e.CompressedSize != uint64(len(f.data)) {
				t.Errorf("sizes = %d/%d, want %d", e.OriginalSize, e.CompressedSize, len(f.data))
			}
			if e.CompressionMethod != None {
				t.Errorf("method = %d, want None", e.CompressionMethod)
			}
			if e.Checksum != crc32.ChecksumIEEE(f.data) {
				t.Errorf("checksum = %08x, want %08x", e.Checksum, crc32.ChecksumIEEE(f.data))
			}
			if e.DataOffset != uint64(HeaderSize) {
				t.Errorf("offset = %d, want %d", e.DataOffset, HeaderSize)
			}
			if ftr.DirectoryOffset != uint64(HeaderSize+len(f.data)) {
				t.Errorf("directory offset = %d, want %d", ftr.DirectoryOffset, HeaderSize+len(f.data))
			}
			if got := b[e.DataOffset : e.DataOffset+e.CompressedSize]; !bytes.Equal(got, f.data) {
				t.Errorf("archived data differs from source (%d vs %d bytes)", len(got), len(f.data))
			}
		})
	}
}

func TestAddFile_MultipleFiles(t *testing.T) {
	dir := chdirTemp(t)
	files := []blob{
		{"first.txt", []byte("first")},
		{"empty.dat", []byte{}},
		{"third.bin", []byte{0, 1, 2, 0, 255, 0}},
		{"fourth.txt", seededBytes(9, 5000)},
	}
	path := buildWithWriter(t, "multi", files)
	b := readTestFile(t, path)
	_, entries, ftr := parseArchiveBytes(t, b)

	if len(entries) != len(files) || ftr.EntryCount != uint32(len(files)) {
		t.Fatalf("entries=%d footer count=%d, want %d", len(entries), ftr.EntryCount, len(files))
	}

	offset := uint64(HeaderSize)
	seenOffsets := map[uint64]string{}
	for i, f := range files {
		e := entries[i]
		if e.Filename != f.name {
			t.Errorf("entry %d name = %q, want %q (order must match AddFile order)", i, e.Filename, f.name)
		}
		if e.DataOffset != offset {
			t.Errorf("entry %d offset = %d, want %d", i, e.DataOffset, offset)
		}
		if e.OriginalSize != uint64(len(f.data)) || e.CompressedSize != uint64(len(f.data)) {
			t.Errorf("entry %d sizes = %d/%d, want %d", i, e.OriginalSize, e.CompressedSize, len(f.data))
		}
		if e.Checksum != crc32.ChecksumIEEE(f.data) {
			t.Errorf("entry %d checksum mismatch", i)
		}
		if got := b[e.DataOffset : e.DataOffset+e.CompressedSize]; !bytes.Equal(got, f.data) {
			t.Errorf("entry %d data mismatch", i)
		}
		// Offsets only need to be distinct for non-empty files.
		if len(f.data) > 0 {
			if prev, dup := seenOffsets[e.DataOffset]; dup {
				t.Errorf("entry %d shares offset %d with %s", i, e.DataOffset, prev)
			}
			seenOffsets[e.DataOffset] = f.name
		}
		offset += uint64(len(f.data))
	}
	if ftr.DirectoryOffset != offset {
		t.Errorf("directory offset = %d, want %d", ftr.DirectoryOffset, offset)
	}
	_ = dir
}

func TestAddFile_UsesBaseNameOfNestedPath(t *testing.T) {
	dir := chdirTemp(t)
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	src := writeTestFile(t, nested, "deep.txt", []byte("deep"))

	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("nested"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(src); err != nil {
		t.Fatal(err)
	}
	if got := w.Files[0].Filename; got != "deep.txt" {
		t.Fatalf("stored name = %q, want base name only", got)
	}
	if strings.ContainsAny(w.Files[0].Filename, `/`) {
		t.Fatal("stored name contains a path separator")
	}
}

// Two inputs with the same base name from different directories: the current
// implementation accepts both and records two entries with the same Filename.
// This test pins that behaviour; whether it is intended is a design decision.
func TestAddFile_DuplicateBaseNames(t *testing.T) {
	dir := chdirTemp(t)
	for _, d := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p1 := writeTestFile(t, filepath.Join(dir, "one"), "same.txt", []byte("AAAA"))
	p2 := writeTestFile(t, filepath.Join(dir, "two"), "same.txt", []byte("BBBBBB"))

	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("dups"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(p1); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(p2); err != nil {
		t.Fatalf("second file with same base name rejected: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := readTestFile(t, filepath.Join(dir, "dups.fz"))
	_, entries, _ := parseArchiveBytes(t, b)
	if len(entries) != 2 || entries[0].Filename != entries[1].Filename {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].DataOffset == entries[1].DataOffset {
		t.Fatal("duplicate-name entries share a data offset")
	}
	if got := b[entries[1].DataOffset : entries[1].DataOffset+entries[1].CompressedSize]; string(got) != "BBBBBB" {
		t.Fatalf("second entry data = %q", got)
	}
	t.Log("note: duplicate base names are stored as separate entries; the reader can only extract the first by name")
}

func TestAddFile_MissingInput(t *testing.T) {
	dir := chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("miss"); err != nil {
		t.Fatal(err)
	}

	if err := w.AddFile(filepath.Join(dir, "does-not-exist.txt")); err == nil {
		t.Fatal("expected error for missing input")
	}
	if len(w.Files) != 0 {
		t.Fatalf("failed AddFile recorded %d entries", len(w.Files))
	}
	if w.Status != Active {
		t.Fatalf("Status = %d, want Active", w.Status)
	}
	info, err := w.OutputFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(HeaderSize) {
		t.Fatalf("output grew to %d bytes after failed AddFile", info.Size())
	}

	// The writer must remain usable.
	src := writeTestFile(t, dir, "ok.txt", []byte("ok"))
	if err := w.AddFile(src); err != nil {
		t.Fatalf("AddFile after failure: %v", err)
	}
	if w.Files[0].DataOffset != uint64(HeaderSize) {
		t.Fatalf("offset after recovered failure = %d, want %d", w.Files[0].DataOffset, HeaderSize)
	}
}

func TestAddFile_FilenameTooLong(t *testing.T) {
	dir := chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("long"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(filepath.Join(dir, strings.Repeat("n", 256))); err == nil {
		t.Fatal("expected error for 256-byte base name")
	}
	if len(w.Files) != 0 || w.Status != Active {
		t.Fatalf("state changed: files=%d status=%d", len(w.Files), w.Status)
	}
}

func TestAddFile_MaxLengthFilename(t *testing.T) {
	dir := chdirTemp(t)
	name := strings.Repeat("m", 255)
	src := filepath.Join(dir, name)
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Skipf("filesystem does not allow 255-byte names: %v", err)
	}
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("max"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(src); err != nil {
		t.Fatalf("255-byte name rejected: %v", err)
	}
	if w.Files[0].Filename != name || w.Files[0].FilenameLength != 255 {
		t.Fatalf("stored entry = %+v", w.Files[0])
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close with 255-byte name: %v", err)
	}
}

// Feeding a directory makes io.Copy fail after the output has been touched
// (on platforms where a directory can be opened). The writer is expected to
// roll back and stay usable.
func TestAddFile_FailedCopyRollsBack(t *testing.T) {
	dir := chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("rollback"); err != nil {
		t.Fatal(err)
	}
	good1 := writeTestFile(t, dir, "good1.txt", []byte("11111"))
	if err := w.AddFile(good1); err != nil {
		t.Fatal(err)
	}
	sizeBefore := int64(HeaderSize + 5)

	bad := filepath.Join(dir, "adirectory")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFile(bad); err == nil {
		t.Fatal("expected error adding a directory")
	}
	if len(w.Files) != 1 {
		t.Fatalf("failed AddFile left %d entries, want 1", len(w.Files))
	}
	if w.Status != Active {
		t.Fatalf("Status = %d, want Active", w.Status)
	}
	info, err := w.OutputFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != sizeBefore {
		t.Fatalf("output size = %d after failed AddFile, want %d", info.Size(), sizeBefore)
	}

	good2 := writeTestFile(t, dir, "good2.txt", []byte("2222222"))
	if err := w.AddFile(good2); err != nil {
		t.Fatalf("AddFile after failed copy: %v", err)
	}
	if w.Files[1].DataOffset != uint64(sizeBefore) {
		t.Fatalf("second offset = %d, want %d", w.Files[1].DataOffset, sizeBefore)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := readTestFile(t, filepath.Join(dir, "rollback.fz"))
	_, entries, _ := parseArchiveBytes(t, b)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	for i, want := range []string{"11111", "2222222"} {
		e := entries[i]
		if got := string(b[e.DataOffset : e.DataOffset+e.CompressedSize]); got != want {
			t.Errorf("entry %d data = %q, want %q", i, got, want)
		}
	}
}

// If the output handle breaks, AddFile and Close must report errors rather
// than recording metadata for data that was never written.
func TestArchiveWriter_BrokenOutputHandle(t *testing.T) {
	dir := chdirTemp(t)
	src := writeTestFile(t, dir, "x.txt", []byte("payload"))
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("broken"); err != nil {
		t.Fatal(err)
	}
	if err := w.OutputFile.Close(); err != nil {
		t.Fatal(err)
	}

	if err := w.AddFile(src); err == nil {
		t.Fatal("AddFile succeeded with a closed output file")
	}
	if len(w.Files) != 0 {
		t.Fatalf("AddFile recorded %d entries despite failing", len(w.Files))
	}
	if err := w.Close(); err == nil {
		t.Fatal("Close succeeded with a closed output file")
	}
	if w.Status == Closed {
		t.Fatal("Status = Closed even though Close failed")
	}
}

func TestClose_EmptyArchive(t *testing.T) {
	dir := chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("empty"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b := readTestFile(t, filepath.Join(dir, "empty.fz"))
	if len(b) != HeaderSize+FooterSize {
		t.Fatalf("size = %d, want %d", len(b), HeaderSize+FooterSize)
	}
	_, entries, ftr := parseArchiveBytes(t, b)
	if len(entries) != 0 {
		t.Fatalf("entries = %d", len(entries))
	}
	want := Footer{Signature: fzFooterSignature, DirectoryOffset: uint64(HeaderSize), DirectorySize: 0, EntryCount: 0}
	if *ftr != want {
		t.Fatalf("footer = %+v, want %+v", *ftr, want)
	}
}

func TestClose_WritesDirectoryAndFooter(t *testing.T) {
	dir := chdirTemp(t)
	files := []blob{
		{"a.txt", []byte("alpha")},
		{"bb.bin", []byte{9, 8, 7, 6}},
		{"ccc", []byte{}},
	}
	path := buildWithWriter(t, "dirfoot", files)
	_ = dir
	b := readTestFile(t, path)
	hdr, entries, ftr := parseArchiveBytes(t, b)

	if hdr.Version != 1 {
		t.Errorf("version = %d", hdr.Version)
	}
	dataLen := 0
	dirLen := 0
	for _, f := range files {
		dataLen += len(f.data)
		dirLen += FileEntryLength + len(f.name)
	}
	if ftr.DirectoryOffset != uint64(HeaderSize+dataLen) {
		t.Errorf("DirectoryOffset = %d, want %d", ftr.DirectoryOffset, HeaderSize+dataLen)
	}
	if ftr.DirectorySize != uint64(dirLen) {
		t.Errorf("DirectorySize = %d, want %d", ftr.DirectorySize, dirLen)
	}
	if ftr.EntryCount != uint32(len(files)) {
		t.Errorf("EntryCount = %d, want %d", ftr.EntryCount, len(files))
	}
	if total := HeaderSize + dataLen + dirLen + FooterSize; len(b) != total {
		t.Errorf("archive size = %d, want %d", len(b), total)
	}
	if len(entries) != len(files) {
		t.Fatalf("entries = %d", len(entries))
	}
	for i, f := range files {
		if entries[i].Filename != f.name {
			t.Errorf("entry %d = %q, want %q", i, entries[i].Filename, f.name)
		}
	}
}

func TestClose_ClosesFileHandle(t *testing.T) {
	chdirTemp(t)
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive("handle"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.OutputFile.Write([]byte{0}); err == nil {
		t.Fatal("output file still writable after Close")
	}
}
