package archive

import (
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

type JobDecompress struct {
	Entry FileEntry
}

func DecompressFile(entry *FileEntry, section *io.SectionReader) error {
	flateReader := flate.NewReader(section)
	defer flateReader.Close()

	//writing to local dir with same name
	dst, err := os.Create(entry.Filename)
	if err != nil {
		return err
	}

	defer dst.Close()

	hasher := crc32.NewIEEE()
	mw := io.MultiWriter(hasher, dst)

	success := false
	defer func() {
		if !success {
			os.Remove(dst.Name())
		}
	}()

	n, err := io.Copy(mw, flateReader)
	if err != nil {
		return err
	}

	if n != int64(entry.OriginalSize) {
		return fmt.Errorf("DECOMPRESS::wrote %d bytes instead of %d", n, entry.OriginalSize)
	}

	if entry.Checksum != hasher.Sum32() {
		return fmt.Errorf("DECOMPRESS::checksum mismatch. Reported: %d, Expected: %d", entry.Checksum, hasher.Sum32())
	}

	success = true
	return nil
}
