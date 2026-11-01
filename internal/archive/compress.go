package archive

import (
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

type Job struct {
	Path              string
	CompressionMethod CompressionMethod
}

type Result struct {
	Path              string
	Filename          string
	CompressionMethod CompressionMethod
	Reader            *io.PipeReader
	OriginalSize      uint64
	Metadata          chan CompressionMetadata
}

type CompressionMetadata struct {
	CompressedSize uint64
	Checksum       uint32
	Err            error
}

type CountWriter struct {
	writer       io.Writer
	bytesWritten int64
}

func (counter *CountWriter) Write(p []byte) (n int, err error) {

	n, err = counter.writer.Write(p)
	counter.bytesWritten += int64(n)
	return
}

func CompressFile(job Job) Result {

	path := job.Path

	res := Result{Path: path}

	file, err := os.Open(path)
	if err != nil {
		return res
	}

	fileInfo, infoErr := file.Stat()
	if infoErr != nil {
		file.Close()
		return res
	}

	res.OriginalSize = uint64(fileInfo.Size())

	pipeReader, pipeWriter := io.Pipe()

	res.Reader = pipeReader
	res.Metadata = make(chan CompressionMetadata, 1)
	go compressionHelper(pipeWriter, job.CompressionMethod, file, res.Metadata, res.OriginalSize)

	return res //return with pipeReader only
}

func compressionHelper(pipeWriter *io.PipeWriter,
	level CompressionMethod,
	file *os.File,
	res chan CompressionMetadata,
	originalSize uint64) {

	defer file.Close()
	metadata := CompressionMetadata{}

	defer func() {
		res <- metadata
	}()

	success := false
	defer func() {
		if !success {
			pipeWriter.CloseWithError(metadata.Err)
		} else {
			pipeWriter.Close()
		}
	}()

	counter := &CountWriter{writer: pipeWriter}
	flateWriter, err := flate.NewWriter(counter, int(level))
	if err != nil {
		metadata.Err = err
		return
	}

	hasher := crc32.NewIEEE()
	multiWriter := io.MultiWriter(hasher, flateWriter)

	n, copyErr := io.Copy(multiWriter, file)
	if copyErr != nil || n != int64(originalSize) {
		if copyErr != nil {
			metadata.Err = copyErr
			return
		}

		metadata.Err = fmt.Errorf("COMPRESS::read %d bytes instead of %d", n, originalSize)
		return
	}

	if err := flateWriter.Close(); err != nil {
		metadata.Err = err
		return
	}

	metadata.Checksum = hasher.Sum32()
	metadata.CompressedSize = uint64(counter.bytesWritten)

	success = true
}
