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

type JobCompress struct {
	Path              string
	CompressionMethod CompressionMethod
}

type ResultCompress struct {
	Path              string
	TempPath          string
	Filename          string
	OriginalSize      uint64
	CompressedSize    uint64
	Checksum          uint32
	CompressionMethod CompressionMethod
	Err               error
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

func CompressFile(job JobCompress) (res ResultCompress) {

	res.Path = job.Path
	res.CompressionMethod = job.CompressionMethod

	fileName := filepath.Base(res.Path)
	if !checkFilename(fileName) {
		res.Err = fmt.Errorf("COMPRESS::invalid filename [%s]", fileName)
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
		res.Err = fmt.Errorf("COMPRESS::error creating temporary file")
		return
	}

	success := false
	defer func() {
		if err := temp.Close(); err != nil {
			res.Err = errors.Join(res.Err, err)
			success = false
		}

		if !success {
			if err := os.Remove(temp.Name()); err != nil {
				res.Err = errors.Join(res.Err, err)
			}
		}

	}()
	res.TempPath = temp.Name()

	counter := &CountWriter{writer: temp}
	hasher := crc32.NewIEEE()

	switch job.CompressionMethod {
	case None:
		multiWriter := io.MultiWriter(hasher, counter)

		n, copyErr := io.Copy(multiWriter, file)
		if copyErr != nil || n != int64(res.OriginalSize) {
			if copyErr != nil {
				res.Err = copyErr
				return
			}

			res.Err = fmt.Errorf("COMPRESS::read %d bytes instead of %d", n, res.OriginalSize)
			return
		}
	default:
		flateWriter, err := flate.NewWriter(counter, int(job.CompressionMethod))
		if err != nil {
			res.Err = err
			return
		}

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
	}

	res.Checksum = hasher.Sum32()
	res.CompressedSize = uint64(counter.bytesWritten)

	success = true

	return res
}
