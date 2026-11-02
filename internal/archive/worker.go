package archive

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

func CompressionWorker(
	jobs <-chan JobCompress,
	results chan<- ResultCompress,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {
		res := CompressFile(job)
		results <- res
	}
}

func StartCompressionPool(
	jobsList []JobCompress,
	workerCount int) <-chan ResultCompress {

	wg := sync.WaitGroup{}

	queueCapacity := 5 //arbitrary rn
	jobs := make(chan JobCompress, queueCapacity)
	res := make(chan ResultCompress, queueCapacity)

	for range workerCount {
		wg.Add(1)
		go CompressionWorker(jobs, res, &wg)
	}

	for _, job := range jobsList {
		jobs <- job
	}

	close(jobs)

	// goroutine that closes results,
	// ultimately closing the archive writer
	go func() {
		wg.Wait()
		close(res)
	}()

	return res
}

func appendResult(
	writer *ArchiveWriter,
	res ResultCompress,
	dataOffset uint64,
	tmp *os.File) (err error) {
	ok := false

	//cleanup function
	defer func() {

		os.Remove(tmp.Name())
		tmp.Close()

		if !ok {

			rollbackErr := writer.OutputFile.Truncate(int64(dataOffset))
			if rollbackErr != nil {
				writer.Status = Failed
				err = errors.Join(err, rollbackErr)
			}

			_, seekErr := writer.OutputFile.Seek(int64(dataOffset), io.SeekStart)
			if seekErr != nil {
				//memory truncated but file pointer not moved
				writer.Status = Failed
				err = errors.Join(err, seekErr)
			}
		}
	}()

	n, copyErr := io.Copy(writer.OutputFile, tmp)
	if copyErr != nil {
		return copyErr
	}

	if n != int64(res.CompressedSize) {
		return fmt.Errorf("WORKER::wrote %d bytes instead of %d", n, res.CompressedSize)

	}

	ok = true
	return nil
}

func WriteArchive(
	writer *ArchiveWriter,
	results <-chan ResultCompress) error {

	if writer.Status != Active {
		return fmt.Errorf("WORKER::invalid writer status: %d", writer.Status)
	}

	var firstErr error

	for res := range results {
		if res.Err != nil {

			if firstErr == nil {
				firstErr = res.Err
			}
			continue
		}

		if firstErr != nil {
			os.Remove(res.TempPath)
			continue
		}

		if len(res.Filename) > 255 {
			firstErr = fmt.Errorf("WORKER::filename too long: %s", res.Filename)
			continue
		}

		tmp, tmpErr := os.Open(res.TempPath)
		if tmpErr != nil {
			firstErr = tmpErr
			os.Remove(res.TempPath)
			continue
		}

		outInfo, outErr := writer.OutputFile.Stat()
		if outErr != nil {
			firstErr = outErr
			os.Remove(res.TempPath)
			continue
		}

		dataOffset := uint64(outInfo.Size())

		if err := appendResult(writer, res, dataOffset, tmp); err != nil {
			firstErr = err
			os.Remove(res.TempPath)
			continue
		}

		entry := FileEntry{
			FilenameLength:    uint8((len(res.Filename))),
			Filename:          res.Filename,
			DataOffset:        dataOffset,
			OriginalSize:      res.OriginalSize,
			CompressedSize:    res.CompressedSize,
			CompressionMethod: res.CompressionMethod,
			Checksum:          res.Checksum,
		}

		writer.Files = append(writer.Files, entry)
	}

	return firstErr
}

func DecompressionWorker(
	jobs <-chan JobDecompress,
	errors chan<- error,
	archive *os.File,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {
		entry := job.Entry
		section := io.NewSectionReader(
			archive,
			int64(entry.DataOffset),
			int64(entry.CompressedSize))

		if err := DecompressFile(&entry, section); err != nil {
			errors <- err
		}
	}
}

func StartDecompressionPool(
	jobsList []JobDecompress,
	reader *ArchiveReader,
	workerCount int) {

	wg := sync.WaitGroup{}

	queueCapacity := 5 //arbitrary rn
	jobs := make(chan JobDecompress, queueCapacity)
	errors := make(chan error, len(jobsList))

	for range workerCount {
		wg.Add(1)
		go DecompressionWorker(jobs, errors, reader.OutputFile, &wg)
	}

	for _, job := range jobsList {
		jobs <- job
	}

	close(jobs)

	wg.Wait()

	for err := range errors {
		fmt.Println(err)
	}
}
