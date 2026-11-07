package archive

import (
	"bytes"
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
	ArchivePath       string
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

	fileName := job.ArchivePath
	if fileName == "" {
		fileName = filepath.Base(job.Path)
	}

	res.Filename = filepath.ToSlash(fileName)

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

func CompressChunk(job ChunkJob) (ChunkResult, error) {
	result := ChunkResult{
		FileID:            job.FileID,
		ChunkIndex:        job.ChunkIndex,
		OriginalSize:      uint64(len(job.Data)),
		CompressionMethod: job.Method,
		Checksum:          crc32.ChecksumIEEE(job.Data),
	}

	if job.Method == None {
		result.Data = job.Data
		result.CompressedSize = uint64(len(job.Data))
		return result, nil
	}

	var buffer bytes.Buffer

	compressor, err := flate.NewWriter(&buffer, int(job.Method))
	if err != nil {
		return ChunkResult{}, err
	}

	if _, err := compressor.Write(job.Data); err != nil {
		return ChunkResult{}, err
	}

	if err := compressor.Close(); err != nil {
		return ChunkResult{}, err
	}

	result.Data = buffer.Bytes()
	result.CompressedSize = uint64(buffer.Len())

	return result, nil
}
