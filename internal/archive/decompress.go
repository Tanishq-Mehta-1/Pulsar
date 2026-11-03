package archive

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

type JobDecompress struct {
	Entry FileEntry
}

func DecompressFile(entry *FileEntry, section *io.SectionReader) (err error) {
	if len(entry.Filename) > 255 {
		return fmt.Errorf("DECOMPRESS::filename is too long for file: %s", entry.Filename)
	}

	dir := filepath.Dir(entry.Filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dst, err := os.Create(entry.Filename)
	if err != nil {
		return err
	}

	defer dst.Close()

	success := false
	defer func() {
		if !success {
			if rmErr := os.Remove(dst.Name()); err != nil {
				err = errors.Join(err, fmt.Errorf("DECOMPRESS::error deleting file: %s after failed extraction", dst.Name()), rmErr)
			}
		}
	}()

	hasher := crc32.NewIEEE()
	mw := io.MultiWriter(hasher, dst)

	var n int64
	switch entry.CompressionMethod {

	case None:
		n, err = io.Copy(mw, section)
		if err != nil {
			return err
		}

	case Default, BestCompression, BestSpeed, HuffmanOnly:
		flateReader := flate.NewReader(section)
		defer flateReader.Close()

		n, err = io.Copy(mw, flateReader)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("DECOMPRESS::incorrect compression method: %d", entry.CompressionMethod)
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
