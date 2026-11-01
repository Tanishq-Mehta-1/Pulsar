package archive

import (
	"sync"
)

func Worker(
	jobs <-chan Job,
	results chan<- Result,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	for job := range jobs {
		res := CompressFile(job)
		results <- res
	}
}

func StartWorkerPool(
	jobsList []Job,
	workerCount int) <-chan Result {

	wg := sync.WaitGroup{}

	queueCapacity := 5 //arbitrary rn
	jobs := make(chan Job, queueCapacity)
	res := make(chan Result, queueCapacity)

	for range workerCount {
		wg.Add(1)
		go Worker(jobs, res, &wg)
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

// // need a better name

// // assuming the writer has already opened the archive and is now
// // writing to the archive

// // assuming that the corresponding entry has the originalSize, filename
// // and method written
// func ArchiveWrite(
// 	results <-chan Result,
// 	writer *ArchiveWriter,
// ) error {

// 	if writer.Status != Active {
// 		return fmt.Errorf("WORKER::archive writer in invalid state: %s", writer.Status)
// 	}

// 	for res := range results {

// 		outputInfo, err := writer.OutputFile.Stat()
// 		if err != nil {
// 			return fmt.Errorf("WRITER::error reading output file stats: %v", err)
// 		}

// 		dataOffset := uint64(outputInfo.Size())

// 		ok := false

// 		defer func() {
// 			if !ok {
// 				//redo changes
// 				rollbackErr := writer.OutputFile.Truncate(int64(dataOffset))
// 				if rollbackErr != nil {
// 					writer.Status = Failed
// 					err = errors.Join(err, rollbackErr)
// 				}

// 				_, seekErr := writer.OutputFile.Seek(int64(dataOffset), io.SeekStart)
// 				if seekErr != nil {
// 					//memory truncated but file pointer not moved
// 					writer.Status = Failed
// 					err = errors.Join(err, rollbackErr)
// 				}
// 			}
// 		}()

// 		if len(res.Filename) > 255 {
// 			return fmt.Errorf("WORKER::filename too long: %s", res.Filename)
// 		}

// 		//writing to archive the compressed stream
// 		n, err := io.Copy(writer.OutputFile, res.Reader)
// 		if err != nil {
// 			return err
// 		}

// 		if err := res.Reader.Close(); err != nil {
// 			return fmt.Errorf("WORKER::error closing pipeReader: %s", err)
// 		}

// 		//completely read
// 		metadata := <-res.Metadata
// 		if metadata.Err != nil {
// 			return metadata.Err
// 		}

// 		if n != int64(metadata.CompressedSize) {
// 			return fmt.Errorf("WORKER::read %d bytes instead %d", n, metadata.CompressedSize)
// 		}

// 		fileEntry := FileEntry{
// 			FilenameLength:    uint8(len(res.Filename)),
// 			Filename:          res.Filename,
// 			DataOffset:        dataOffset,
// 			OriginalSize:      res.OriginalSize,
// 			CompressedSize:    metadata.CompressedSize,
// 			CompressionMethod: res.CompressionMethod,
// 			Checksum:          metadata.Checksum,
// 		}

// 		writer.Files = append(writer.Files, fileEntry)

// 		ok = true
// 	}

// 	return nil
// }
