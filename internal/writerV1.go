package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type ArchiveWriterV1 struct {
	OutputFile *os.File
	Files      []FileEntry
	Status     ObjStatus
}

func (writer *ArchiveWriterV1) CreateArchive(fileName string, outputDir string) error {

	if writer.Status != Created {
		return fmt.Errorf("WRITERV1::invalid ArchiveWriterV1 status %d", writer.Status)
	}

	if !checkFilename(fileName) {
		return errors.New("WRITERV1::invalid filename")
	}

	if fileName == "" {
		fileName = "output"
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

	// assign output file to ArchiveWriterV1
	writer.OutputFile = file

	archiveHeader := Header{
		Signature: fzHeaderSignature,
		Version:   1,
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
		return fmt.Errorf("WRITERV1::wrote %d bytes instead of %d. Error: %s", n, len(byte_stream), err)
	}

	writer.Status = Active
	ok = true

	return nil
}

func (writer *ArchiveWriterV1) AddFilesConcurrent(files []FileStruct, numWorkers int) error {

	if writer.Status != Active {
		return fmt.Errorf("WRITERV1::invalid ArchiveWriterV1 status %d", writer.Status)
	}

	if numWorkers <= 0 {
		return fmt.Errorf("WRITERV1::worker count must be positive")
	}

	jobs := make([]JobCompress, 0, len(files))
	for _, file := range files {

		switch file.Method {
		case None, BestCompression, BestSpeed, Default, HuffmanOnly:
		default:
			return fmt.Errorf("WRITERV1::incorrect compression method: %d", file.Method)
		}

		jobs = append(jobs, JobCompress{Path: file.Path, ArchivePath: file.ArchivePath, CompressionMethod: file.Method})
	}

	res := StartCompressionPool(jobs, numWorkers)

	if err := WriteArchiveV1(writer, res); err != nil {
		return fmt.Errorf("WRITERV1::error writing to archive concurrently: %w", err)
	}

	return nil
}

func (writer *ArchiveWriterV1) AddDirectory(dirPath string, numWorkers int) error {

	if !dirExists(dirPath) {
		return fmt.Errorf("WRITERV1::error opening the directory")
	}

	if numWorkers <= 0 {
		return fmt.Errorf("WRITERV1::numWorkers must be positive")
	}

	var files []FileStruct
	var directories []FileEntry

	if err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {

		if err != nil {
			return fmt.Errorf("WRITERV1::error accessing path %q: %w", path, err)
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {

			return err
		}

		root := filepath.Base(filepath.Clean(dirPath))
		relPath = filepath.ToSlash(filepath.Join(root, relPath))

		if d.IsDir() {

			directories = append(directories, FileEntry{
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

func (writer *ArchiveWriterV1) Close() error {

	if writer.Status != Active {
		return fmt.Errorf("WRITERV1::invalid ArchiveWriterV1 status %d", writer.Status)
	}

	if len(writer.Files) > int(MAXUINT32) {
		return fmt.Errorf("WRITERV1::file count %d exceeds limit", len(writer.Files))
	}

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV1::error reading output file stats: %v", err)
	}

	directoryOffset := uint64(outputInfo.Size())

	entryCount := uint32(len(writer.Files))

	success := false
	defer func() {
		if !success {
			writer.Status = Failed
		}
	}()

	//write the FileEntries to file
	for _, fileEntry := range writer.Files {

		buffer, err := SerializeFileEntry(&fileEntry)
		if err != nil {
			return err
		}

		n, err := writer.OutputFile.Write(buffer)
		if err != nil || n != len(buffer) {
			return fmt.Errorf("WRITERV1::wrote %d bytes instead of %d bytes. Error: %s", n, len(buffer), err)
		}
	}

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV1::error reading output file stats: %v", err)
	}

	directorySize := uint64(outputInfo.Size()) - directoryOffset

	//write the Footer to file
	archiveFooter := FooterV1{
		Signature:       fzFooterSignature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount,
	}

	buffer := SerializeFooterV1(&archiveFooter)

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITERV1::error reading output file stats: %v", err)
	}

	//validate footer
	if directoryOffset+directorySize != uint64(outputInfo.Size()) {
		return errors.New("WRITERV1::invalid directory offset or size")
	}

	n, err := writer.OutputFile.Write(buffer[:])
	if err != nil || n != len(buffer) {
		return fmt.Errorf("WRITERV1::wrote %d bytes instead of %d. Error: %s", n, len(buffer), err)
	}

	if err := writer.OutputFile.Close(); err != nil {
		return err
	}

	writer.Status = Closed
	success = true

	return nil
}
