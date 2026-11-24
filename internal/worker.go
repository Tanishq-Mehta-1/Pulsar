package archive

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const KB = 1024

const queueCapacity int = 100 //arbitrary rn

func appendResult(
	writer *ArchiveWriterV1,
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

func WriteArchiveV1(
	writer *ArchiveWriterV1,
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
	workerCount int,
) <-chan ResultCompress {

	var wg sync.WaitGroup

	jobs := make(chan JobCompress, queueCapacity)
	results := make(chan ResultCompress, queueCapacity)

	for range workerCount {
		wg.Add(1)
		go CompressionWorker(jobs, results, &wg)
	}

	// Feed jobs asynchronously.
	go func() {
		defer close(jobs)

		for _, job := range jobsList {
			jobs <- job
		}
	}()

	// Close results after all workers finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func DecompressionWorker(
	jobs <-chan JobDecompress,
	errors chan<- error,
	archive *os.File,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {

		if job.Entry.Type == Directory {
			if err := os.MkdirAll(job.Entry.Filename, 0755); err != nil {
				fmt.Println("Decompression error:", err)
				errors <- err
			}
			continue
		}

		entry := job.Entry
		section := io.NewSectionReader(
			archive,
			int64(entry.DataOffset),
			int64(entry.CompressedSize))

		// fmt.Println("Decompressing:", job.Entry.Filename)

		if err := DecompressFile(&entry, section); err != nil {
			fmt.Println("Decompression error:", err)
			errors <- err
		}
	}
}

func StartDecompressionPool(
	jobsList []JobDecompress,
	reader *ArchiveReader,
	workerCount int) error {

	if reader.Version != 1 {
		return fmt.Errorf("WORKER::invalid reader version : %d\n", reader.Version)
	}

	wg := sync.WaitGroup{}

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

	go func() {
		wg.Wait()
		close(errors)
	}()

	var firstErr error
	for err := range errors {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func ChunkCompressionWorker(
	jobs <-chan ChunkJob,
	results chan<- ChunkResult,
	errors chan<- error,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {
		res, err := CompressChunk(job)
		if job.Method != None || err != nil {
			putChunkBuffer(res.Data)
		}

		if err != nil {
			fmt.Println("error while compressing chunk:", err)
			errors <- err
			continue
		}

		results <- res
	}
}

func StartChunkCompressionPool(
	files []FileStruct,
	workerCount int,
	writer *ArchiveWriterV2,
	errors chan error,
) (<-chan ChunkResult, <-chan []FileEntryChunk) {

	var wg sync.WaitGroup

	jobs := make(chan ChunkJob, queueCapacity)
	results := make(chan ChunkResult, queueCapacity)

	fileEntryMetadata := make(chan []FileEntryChunk, 1)

	//start job chunker
	wg.Add(1)
	go writer.generateChunks(files, jobs, fileEntryMetadata, errors, &wg, workerCount)

	for range workerCount {
		wg.Add(1)
		go ChunkCompressionWorker(jobs, results, errors, &wg)
	}

	// Close results and errors after all workers finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	return results, fileEntryMetadata
}

func writeChunk(
	writer *ArchiveWriterV2,
	res ChunkResult,
	dataOffset uint64,
) (err error) {
	ok := false

	//cleanup function
	defer func() {

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

	n, writeErr := writer.OutputFile.Write(res.Data)
	if writeErr != nil {
		return writeErr
	}

	if n != int(res.CompressedSize) {
		return fmt.Errorf("WORKER::wrote %d bytes instead of %d", n, res.CompressedSize)

	}

	ok = true

	return nil
}

func WriteArchiveV2(
	writer *ArchiveWriterV2,
	results <-chan ChunkResult,
	wg *sync.WaitGroup,
	errors chan<- error,
) {

	defer wg.Done()

	if writer.Status != Active {
		errors <- fmt.Errorf("WORKER::invalid writer status: %d", writer.Status)

	}

	var firstErr error
	for res := range results {

		if firstErr != nil {
			putChunkBuffer(res.Data)
			continue
		}

		dataOffset, offErr := writer.OutputFile.Seek(0, io.SeekCurrent)
		if offErr != nil {
			putChunkBuffer(res.Data)
			firstErr = offErr
			continue
		}

		if err := writeChunk(writer, res, uint64(dataOffset)); err != nil {
			putChunkBuffer(res.Data)
			firstErr = err
			continue
		}

		if res.CompressionMethod == None {
			putChunkBuffer(res.Data)
		}

		chunkEntry := ChunkEntry{
			FileID:            res.FileID,
			ChunkIndex:        res.ChunkIndex,
			DataOffset:        uint64(dataOffset),
			OriginalSize:      res.OriginalSize,
			CompressedSize:    res.CompressedSize,
			CompressionMethod: res.CompressionMethod,
			Checksum:          res.Checksum,
		}

		writer.Chunks = append(writer.Chunks, chunkEntry)
	}

	errors <- firstErr
}

func ChunkDecompressionWorker(
	jobs <-chan JobDecompressChunk,
	errors chan<- error,
	archive *os.File,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {

		if err := DecompressChunk(job, archive); err != nil {
			errors <- err
		}
	}
}

func StartChunkDecompressionPool(
	jobsList []JobDecompressChunk,
	reader *ArchiveReader,
	workerCount int,
	archive *os.File) error {

	if reader.Version != 2 {
		return fmt.Errorf("WORKER::invalid reader version : %d\n", reader.Version)
	}

	wg := sync.WaitGroup{}

	jobs := make(chan JobDecompressChunk, queueCapacity)
	errors := make(chan error, len(jobsList))

	for range workerCount {
		wg.Add(1)
		go ChunkDecompressionWorker(jobs, errors, archive, &wg)
	}

	for _, job := range jobsList {
		jobs <- job
	}

	close(jobs)

	go func() {
		wg.Wait()
		close(errors)
	}()

	var firstErr error
	for err := range errors {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func calculateChunkSize(size int64, workerCount int) (error, uint32) {
	target := size / int64(workerCount*2)

	if target < 128*KB {
		return nil, 64 * KB
	} else if target >= 128*KB && target < 256*KB {
		return nil, 128 * KB
	} else if target >= 256*KB && target < 512*KB {
		return nil, 256 * KB
	} else if target >= 512*KB && target < 1024*KB {
		return nil, 512 * KB
	} else if target >= 1024*KB && target < 2048*KB {
		return nil, 1024 * KB
	} else if target >= 2048*KB && target < 4096*KB {
		return nil, 2048 * KB
	} else {
		return nil, 4096 * KB
	}
}
