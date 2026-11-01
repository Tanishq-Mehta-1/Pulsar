package archive

import (
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

type Job struct {
	Path              string
	FileID            uint32
	CompressionMethod CompressionMethod
}

type Result struct {
	FileID            uint32
	Path              string
	TempPath          string
	Filename          string
	OriginalSize      uint64
	CompressedSize    uint64
	Checksum          uint32
	CompressionMethod CompressionMethod
	Err               error
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

func CompressFile(job Job) (res Result) {

	res.Path = job.Path
	res.CompressionMethod = job.CompressionMethod
	res.FileID = job.FileID

	fileName := filepath.Base(res.Path)
	if !checkFilename(fileName) {
		res.Err = fmt.Errorf("WORKER::invalid filename [%s]", fileName)
		return
	}
	res.Filename = fileName

	file, err := os.Open(job.Path)
	if err != nil {
		res.Err = err
		return res
	}

	defer file.Close()

	fileInfo, infoErr := file.Stat()
	if infoErr != nil {
		res.Err = infoErr
		return
	}
	res.OriginalSize = uint64(fileInfo.Size())

	temp, err := os.CreateTemp("", "pulsar-*")
	if err != nil {
		res.Err = fmt.Errorf("WORKER::error creating temporary file")
		return
	}

	success := false
	defer func() {
		if err := temp.Close(); err != nil {
			res.Err = err
		}

		if !success {
			if err := os.Remove(temp.Name()); err != nil {
				res.Err = err
			}
		}

	}()
	res.TempPath = temp.Name()

	counter := &CountWriter{writer: temp}
	flateWriter, err := flate.NewWriter(counter, int(job.CompressionMethod))
	if err != nil {
		res.Err = err
		return
	}

	hasher := crc32.NewIEEE()
	multiWriter := io.MultiWriter(hasher, flateWriter)

	n, copyErr := io.Copy(multiWriter, file)
	if copyErr != nil || n != int64(res.OriginalSize) {
		if copyErr != nil {
			res.Err = copyErr
			return
		}

		res.Err = fmt.Errorf("COMPRESS::read %d bytes instead of %d", n, res.OriginalSize)
		return
	}

	if err := flateWriter.Close(); err != nil {
		res.Err = err
		return
	}

	res.Checksum = hasher.Sum32()
	res.CompressedSize = uint64(counter.bytesWritten)

	success = true

	return res
}
