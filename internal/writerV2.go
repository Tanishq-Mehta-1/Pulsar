package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type ArchiveWriterV2 struct {
	OutputFile *os.File
	Files      []FileEntryChunk
	Chunks     []ChunkEntry
	ChunkSize  uint32
	NextFileID uint32
	Status     ObjStatus
}

const MAXUINT32 uint32 = 4294967295
const MAXCHUNKSIZE uint32 = 6.4e7

func (writer *ArchiveWriterV2) CreateArchive(
	fileName string,
	outputDir string,
	chunkSize uint32) error {

	if writer.Status != Created {
		return fmt.Errorf("WRITERV2::invalid ArchiveWriterV2 status %d", writer.Status)
	}

	if !checkFilename(fileName) {
		return errors.New("WRITERV2::invalid filename")
	}

	if fileName == "" {
		fileName = "output"
	}

	if chunkSize > MAXCHUNKSIZE {
		return fmt.Errorf("WRITERV2::chunk size of %d larger than Max Chunk size allowed: %d", chunkSize, MAXCHUNKSIZE)
	}

	fileName += extension

	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}

		fileName = filepath.Join(outputDir, fileName)
	}

	ok := false

	file, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer func() {
		if !ok {
			file.Close()
			writer.OutputFile = nil
		}
	}()

	// assign output file to ArchiveWriterV2
	writer.OutputFile = file

	archiveHeader := Header{
		Signature: fzHeaderSignature,
		Version:   2,
		Flags:     0,
		Reserved:  0,
	}

	// write header to file
	byte_stream := SerializeHeader(&archiveHeader)

	n, err := file.Write(byte_stream[:])
	if err != nil || n != len(byte_stream) {

		writer.Status = Failed

		if err != nil {
			return err
		}
		return fmt.Errorf("WRITERV2::wrote %d bytes instead of %d. Error: %s", n, len(byte_stream), err)
	}

	writer.Status = Active
	ok = true

	return nil
}

func (writer *ArchiveWriterV2) AddFilesConcurrent(
	files []FileStruct,
	numWorkers int) (err error) {

	if writer.Status != Active {
		return fmt.Errorf("WRITERV2::invalid ArchiveWriterV2 status %d", writer.Status)
	}

	if numWorkers <= 0 {
		return fmt.Errorf("WRITERV2::worker count must be positive")
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(writer.Files))

	res, fileEntries := StartChunkPool(files, numWorkers, writer, errors)

	wg.Add(1)
	go WriteArchiveV2(writer, res, &wg, errors)

	go func() {
		var firstErr error
		for err := range errors {
			if err != nil && firstErr == nil {
				writer.Status = Failed
				fmt.Println(err)
				firstErr = err
			}
		}

		err = firstErr
	}()

	wg.Wait()
	close(errors)

	for _, file := range <-fileEntries {
		writer.Files = append(writer.Files, file)
	}

	return nil
}

func (writer *ArchiveWriterV2) AddDirectory(
	dirPath string,
	numWorkers int) error {

	if !dirExists(dirPath) {
		return fmt.Errorf("WRITERV2::error opening the directory")
	}

	if numWorkers <= 0 {
		return fmt.Errorf("WRITERV2::numWorkers must be positive")
	}

	var files []FileStruct
	var directories []FileEntryChunk

	if err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {

		if err != nil {
			return fmt.Errorf("WRITERV2::error accessing path %q: %w", path, err)
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {

			return err
		}

		root := filepath.Base(filepath.Clean(dirPath))
		relPath = filepath.ToSlash(filepath.Join(root, relPath))

		if d.IsDir() {

			directories = append(directories, FileEntryChunk{
				Filename:       relPath + "/",
				FilenameLength: uint8(len(relPath + "/")),
				Type:           Directory})
			return nil
		}

		files = append(files, FileStruct{
			Path:        path,
			ArchivePath: relPath,
			Method:      Default})

		return nil

	}); err != nil {
		return err
	}

	if err := writer.AddFilesConcurrent(files, numWorkers); err != nil {
		return err
	}

	writer.Files = append(writer.Files, directories...)
	return nil
}

func (writer *ArchiveWriterV2) generateChunks(
	files []FileStruct,
	jobs chan<- ChunkJob,
	metadata chan<- []FileEntryChunk,
	errors chan<- error,
	wg *sync.WaitGroup,
) {

	defer wg.Done()

	var res []FileEntryChunk

	for _, file := range files {

		success := true
		fileId := writer.NextFileID

		if len(file.ArchivePath) > 255 {
			errors <- fmt.Errorf("WRITERV2::archive filename too long")
			continue
		}

		switch file.Method {
		case None, BestCompression, BestSpeed, Default, HuffmanOnly:
		default:
			errors <- fmt.Errorf("WRITERV2::incorrect compression method: %d", file.Method)
			continue
		}

		//open file
		f, err := os.Open(file.Path)
		if err != nil {
			errors <- err
			continue
		}

		info, err := f.Stat()
		if err != nil {
			f.Close()
			errors <- err
			continue
		}
		fileSize := info.Size()

		var chunkCount uint32 = 0
		var i int64

		for i = 0; i < fileSize; i += int64(writer.ChunkSize) {

			var bufSize int = int(min(writer.ChunkSize, uint32(fileSize-i)))
			buffer := make([]byte, bufSize)
			if n, err := f.ReadAt(buffer, i); err != nil || n != bufSize {

				success = false
				f.Close()
				if err != nil {
					errors <- err
					break
				}

				errors <- fmt.Errorf("WRITERV2::wrote %d bytes to chunk instead of %d", n, bufSize)
				break
			}

			chunkJob := ChunkJob{
				FileID:     uint32(fileId),
				ChunkIndex: chunkCount,
				Data:       buffer,
				Method:     file.Method,
			}
			chunkCount++

			jobs <- chunkJob
		}

		if success {
			if err = f.Close(); err != nil {
				errors <- err
				continue
			}

			fChunk := FileEntryChunk{
				FilenameLength: uint8(len(file.ArchivePath)),
				Filename:       file.ArchivePath,
				FileID:         uint32(fileId),
				OriginalSize:   uint64(info.Size()),
				ChunkCount:     chunkCount,
			}

			res = append(res, fChunk)
		}

		writer.NextFileID++

	}

	//when done, close jobs
	close(jobs)

	metadata <- res
	errors <- nil
}

func (writer *ArchiveWriterV2) Close() error {

	if writer.Status != Active {
		return fmt.Errorf("WRITERV2::invalid ArchiveWriterV2 status %d", writer.Status)
	}

	if len(writer.Files) > int(MAXUINT32) {
		return fmt.Errorf("WRITERV2::file count %d exceeds limit", len(writer.Files))
	}

	if len(writer.Chunks) > int(MAXUINT32) {
		return fmt.Errorf("WRITERV2::file count %d exceeds limit", len(writer.Chunks))
	}

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV2::error reading output file stats: %v", err)
	}

	directoryOffset := uint64(outputInfo.Size())

	entryCount := uint32(len(writer.Files))
	chunkCount := uint32(len(writer.Chunks))

	success := false
	defer func() {
		if !success {
			writer.Status = Failed
		}
	}()

	//write the FileEntryChunks to file
	for _, fileEntry := range writer.Files {

		buffer, err := SerializeFileEntryChunk(&fileEntry)
		if err != nil {
			return err
		}

		n, err := writer.OutputFile.Write(buffer)
		if err != nil || n != len(buffer) {
			return fmt.Errorf("WRITERV2::wrote %d bytes instead of %d bytes. Error: %s", n, len(buffer), err)
		}
	}

	//write the ChunkEntries to file
	for _, chunkEntry := range writer.Chunks {

		buffer, err := SerializeChunkEntry(&chunkEntry)
		if err != nil {
			return err
		}

		n, err := writer.OutputFile.Write(buffer)
		if err != nil || n != len(buffer) {
			return fmt.Errorf("WRITERV2::wrote %d bytes instead of %d bytes. Error: %s", n, len(buffer), err)
		}
	}

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV2::error reading output file stats: %v", err)
	}

	directorySize := uint64(outputInfo.Size()) - directoryOffset

	//write the Footer to file
	archiveFooter := FooterV2{
		Signature:       fzFooterSignature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount,
		ChunkCount:      chunkCount,
	}

	buffer := SerializeFooterV2(&archiveFooter)

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV2::error reading output file stats: %v", err)
	}

	//validate footer
	if directoryOffset+directorySize != uint64(outputInfo.Size()) {
		return errors.New("WRITERV2::invalid directory offset or size")
	}

	n, err := writer.OutputFile.Write(buffer[:])
	if err != nil || n != len(buffer) {
		return fmt.Errorf("WRITERV2::wrote %d bytes instead of %d. Error: %s", n, len(buffer), err)
	}

	if err := writer.OutputFile.Close(); err != nil {
		return err
	}

	writer.Status = Closed
	success = true

	return nil
}
