package archive

import (
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// blob is a named chunk of file content used to build archives in tests.
type blob struct {
	name string
	data []byte
}

// chdirTemp switches the working directory to a fresh temporary directory for
// the duration of the test (CreateArchive always writes into the working
// directory) and returns that directory.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restoring working directory: %v", err)
		}
	})
	return dir
}

func writeTestFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return p
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

// closeWriterOnCleanup makes sure a writer's file handle is released even when
// the test fails half way (matters for TempDir removal on some platforms).
func closeWriterOnCleanup(t *testing.T, w *ArchiveWriter) {
	t.Helper()
	t.Cleanup(func() {
		if w.OutputFile != nil {
			_ = w.OutputFile.Close()
		}
	})
}

func closeReaderOnCleanup(t *testing.T, r *ArchiveReader) {
	t.Helper()
	t.Cleanup(func() {
		if r.OutputFile != nil {
			_ = r.OutputFile.Close()
		}
	})
}

func openReader(t *testing.T, path string) *ArchiveReader {
	t.Helper()
	r := &ArchiveReader{}
	closeReaderOnCleanup(t, r)
	if err := r.OpenArchive(path); err != nil {
		t.Fatalf("OpenArchive(%s): %v", path, err)
	}
	return r
}

// capturePanic runs fn and returns the recovered panic value (nil if none).
func capturePanic(fn func()) (p any) {
	defer func() { p = recover() }()
	fn()
	return nil
}

func seededBytes(seed int64, n int) []byte {
	rng := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.Intn(256))
	}
	return b
}

// sampleBlobs returns a spread of content shapes: empty, tiny, text, every
// byte value, zero runs, and sizes beyond a typical copy buffer.
func sampleBlobs() []blob {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	return []blob{
		{"empty.txt", []byte{}},
		{"one.bin", []byte{0x00}},
		{"text.txt", []byte("hello, pulsar\nline two\n")},
		{"allbytes.bin", all},
		{"zeros.bin", make([]byte, 4096)},
		{"random.bin", seededBytes(1, 100_000)},
		{"large.bin", seededBytes(2, (1<<20)+7)},
	}
}

// buildWithWriter creates an archive in the current working directory (see
// chdirTemp) using the real ArchiveWriter and returns its path. Source files
// are staged in a "src" sub-directory.
func buildWithWriter(t *testing.T, archiveName string, files []blob) string {
	t.Helper()
	return buildWithWriterMethod(t, archiveName, files, None)
}

// buildWithWriterMethod is buildWithWriter with an explicit compression method.
func buildWithWriterMethod(t *testing.T, archiveName string, files []blob, method CompressionMethod) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	src := filepath.Join(cwd, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	w := &ArchiveWriter{}
	closeWriterOnCleanup(t, w)
	if err := w.CreateArchive(archiveName); err != nil {
		t.Fatalf("CreateArchive: %v", err)
	}
	for _, f := range files {
		p := writeTestFile(t, src, f.name, f.data)
		if err := w.AddFile(p, method); err != nil {
			t.Fatalf("AddFile(%s): %v", f.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return filepath.Join(cwd, archiveName+extension)
}

// parseArchiveBytes decodes an archive without using ArchiveReader.
func parseArchiveBytes(t *testing.T, b []byte) (*Header, []FileEntry, *Footer) {
	t.Helper()
	if len(b) < HeaderSize+FooterSize {
		t.Fatalf("archive too small: %d bytes", len(b))
	}
	hdr, err := DeserializeHeader([HeaderSize]byte(b[:HeaderSize]))
	if err != nil {
		t.Fatalf("DeserializeHeader: %v", err)
	}
	ftr, err := DeserializeFooter([FooterSize]byte(b[len(b)-FooterSize:]))
	if err != nil {
		t.Fatalf("DeserializeFooter: %v", err)
	}
	end := ftr.DirectoryOffset + ftr.DirectorySize
	if ftr.DirectoryOffset > uint64(len(b)) || end > uint64(len(b)) || end < ftr.DirectoryOffset {
		t.Fatalf("footer directory range [%d,+%d) outside archive of %d bytes",
			ftr.DirectoryOffset, ftr.DirectorySize, len(b))
	}
	dir := b[ftr.DirectoryOffset:end]
	var entries []FileEntry
	for pos := 0; pos < len(dir); {
		e, n, err := DeserializeFileEntry(dir[pos:])
		if err != nil {
			t.Fatalf("DeserializeFileEntry at directory offset %d: %v", pos, err)
		}
		entries = append(entries, *e)
		pos += n
	}
	return hdr, entries, ftr
}

func entryForData(name string, offset uint64, content []byte) FileEntry {
	return FileEntry{
		FilenameLength:    uint8(len(name)),
		Filename:          name,
		DataOffset:        offset,
		OriginalSize:      uint64(len(content)),
		CompressedSize:    uint64(len(content)),
		CompressionMethod: None,
		Checksum:          crc32.ChecksumIEEE(content),
	}
}

// assembleArchive hand-builds [header][data][directory][footer] from the given
// pieces, independent of ArchiveWriter.
func assembleArchive(t *testing.T, data []byte, entries []FileEntry) []byte {
	t.Helper()
	hdr := SerializeHeader(&Header{Signature: fzHeaderSignature, Version: 1})
	out := append([]byte{}, hdr[:]...)
	out = append(out, data...)
	dirOff := len(out)
	for i := range entries {
		b, err := SerializeFileEntry(&entries[i])
		if err != nil {
			t.Fatalf("SerializeFileEntry(%q): %v", entries[i].Filename, err)
		}
		out = append(out, b...)
	}
	ftr := SerializeFooter(&Footer{
		Signature:       fzFooterSignature,
		DirectoryOffset: uint64(dirOff),
		DirectorySize:   uint64(len(out) - dirOff),
		EntryCount:      uint32(len(entries)),
	})
	return append(out, ftr[:]...)
}

// layoutArchive lays blobs out back to back after the header and assembles a
// well-formed archive. It also returns the entries it wrote.
func layoutArchive(t *testing.T, files []blob) ([]byte, []FileEntry) {
	t.Helper()
	var data []byte
	var entries []FileEntry
	for _, f := range files {
		entries = append(entries, entryForData(f.name, uint64(HeaderSize+len(data)), f.data))
		data = append(data, f.data...)
	}
	return assembleArchive(t, data, entries), entries
}

func footerOf(t *testing.T, arch []byte) Footer {
	t.Helper()
	f, err := DeserializeFooter([FooterSize]byte(arch[len(arch)-FooterSize:]))
	if err != nil {
		t.Fatalf("DeserializeFooter: %v", err)
	}
	return *f
}

// patchFooter returns a copy of arch whose footer has been modified by mutate.
func patchFooter(t *testing.T, arch []byte, mutate func(*Footer)) []byte {
	t.Helper()
	out := append([]byte{}, arch...)
	f := footerOf(t, out)
	mutate(&f)
	b := SerializeFooter(&f)
	copy(out[len(out)-FooterSize:], b[:])
	return out
}

// replaceDirectory returns an archive with the same header+data region but a
// different directory, and a footer that is self-consistent with it.
func replaceDirectory(t *testing.T, arch []byte, newDir []byte, count uint32) []byte {
	t.Helper()
	f := footerOf(t, arch)
	out := append([]byte{}, arch[:f.DirectoryOffset]...)
	out = append(out, newDir...)
	nf := SerializeFooter(&Footer{
		Signature:       fzFooterSignature,
		DirectoryOffset: f.DirectoryOffset,
		DirectorySize:   uint64(len(newDir)),
		EntryCount:      count,
	})
	return append(out, nf[:]...)
}

func directoryOf(t *testing.T, arch []byte) []byte {
	t.Helper()
	f := footerOf(t, arch)
	return append([]byte{}, arch[f.DirectoryOffset:f.DirectoryOffset+f.DirectorySize]...)
}

func writeArchiveBytes(t *testing.T, b []byte) string {
	t.Helper()
	return writeTestFile(t, t.TempDir(), "test.fz", b)
}
