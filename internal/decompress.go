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
	Entry     FileEntry
	OutputDir string
}

type JobDecompressChunk struct {
	Chunk     ChunkEntry
	DstFile   *os.File
	Dstoffset int64
}

func DecompressFile(
	entry *FileEntry,
	section *io.SectionReader,
	outputDir ...string) error {
	if len(entry.Filename) > 255 {
		return fmt.Errorf("DECOMPRESS::filename is too long for file: %s", entry.Filename)
	}

	targetPath := filepath.FromSlash(entry.Filename)
	if len(outputDir) > 0 && outputDir[0] != "" {
		targetPath = filepath.Join(outputDir[0], targetPath)
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}

	defer dst.Close()

	success := false
	defer func() {
		if !success {
			if rmErr := os.Remove(dst.Name()); rmErr != nil {
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

	case Zstd:
		n, err = DecompressZstdStream(mw, section)
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

func DecompressChunk(
	job JobDecompressChunk,
	archive *os.File) error {

	var data []byte
	var err error
	sectionReader := io.NewSectionReader(archive, int64(job.Chunk.DataOffset), int64(job.Chunk.CompressedSize))

	switch job.Chunk.Codec {
	case CodecStored:
		data = make([]byte, job.Chunk.CompressedSize)
		n, err := archive.ReadAt(data, int64(job.Chunk.DataOffset))
		if n != int(job.Chunk.CompressedSize) || err != nil {
			if err != nil {
				return err
			}

			return fmt.Errorf("DECOMPRESS::Read %d bytes instead of %d", n, job.Chunk.CompressedSize)
		}

	case CodecDeflate:
		switch job.Chunk.CompressionMethod {
		case None:

			data = make([]byte, job.Chunk.CompressedSize)
			n, err := archive.ReadAt(data, int64(job.Chunk.DataOffset))
			if n != int(job.Chunk.CompressedSize) || err != nil {
				if err != nil {
					return err
				}

				return fmt.Errorf("DECOMPRESS::Read %d bytes instead of %d", n, job.Chunk.CompressedSize)
			}

		case Default, BestCompression, BestSpeed, HuffmanOnly:

			flateReader := flate.NewReader(sectionReader)
			defer flateReader.Close()

			data, err = io.ReadAll(flateReader)
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("DECOMPRESS::invalid chunk compression method: %d, fileID: %d\n", job.Chunk.CompressionMethod, job.Chunk.FileID)
		}
	case CodecZstd:

		dst := getChunkBuffer(int(job.Chunk.OriginalSize))
		defer putChunkBuffer(dst)
		src := getChunkBuffer(int(job.Chunk.CompressedSize))
		defer putChunkBuffer(src)
		n, err := archive.ReadAt(src, int64(job.Chunk.DataOffset))
		if n != int(job.Chunk.CompressedSize) || err != nil {
			if err != nil {
				return err
			}

			return fmt.Errorf("DECOMPRESS::Read %d bytes instead of %d", n, job.Chunk.CompressedSize)
		}

		data, err = DecompressZstd(src, dst)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("DECOMPRESS::invalid codec: %d", job.Chunk.Codec)
	}

	if len(data) != int(job.Chunk.OriginalSize) {
		return fmt.Errorf("DECOMPRESS::invalid uncompressed size of %d, original size: %d", len(data), job.Chunk.OriginalSize)
	}

	if crc32.ChecksumIEEE(data) != job.Chunk.Checksum {
		return fmt.Errorf("DECOMPRESS::invalid checksum for ChunkID: %d, FileID: %d", job.Chunk.ChunkIndex, job.Chunk.FileID)
	}

	n, err := job.DstFile.WriteAt(data, job.Dstoffset)
	if err != nil {
		return fmt.Errorf("DECOMPRESS::%v", err)
	}

	if uint64(n) != job.Chunk.OriginalSize {
		return fmt.Errorf("DECOMPRESS::wrote %d bytes instead of %d", n, job.Chunk.OriginalSize)
	}

	return nil
}
