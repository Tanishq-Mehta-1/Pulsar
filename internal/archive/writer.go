package archive

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var extension string = ".fz"

type ObjStatus uint8

const (
	Created ObjStatus = 0
	Active  ObjStatus = 2
	Closed  ObjStatus = 3
	Failed  ObjStatus = 4
)

type ArchiveWriter struct {
	OutputFile *os.File
	Files      []FileEntry
	Status     ObjStatus
}

type FileStruct struct {
	Path        string
	ArchivePath string
	Method      CompressionMethod
}

func checkFilename(fileName string) bool {
	if len(fileName) > 255 || strings.Contains(fileName, "/") {
		return false
	}

	return true
}

func (writer *ArchiveWriter) CreateArchive(fileName string, outputDir string) error {

	if writer.Status != Created {
		return fmt.Errorf("WRITER::invalid ArchiveWriter status %d", writer.Status)
	}

	if !checkFilename(fileName) {
		return errors.New("WRITER::invalid filename")
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

	// assign output file to ArchiveWriter
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
		return fmt.Errorf("WRITER::wrote %d bytes instead of %d. Error: %s", n, len(byte_stream), err)
	}

	writer.Status = Active
	ok = true

	return nil
}

// returns compressed_size, checksum and error
func (writer *ArchiveWriter) writeCompressed(file *os.File, method CompressionMethod, dataOffset uint64, originalSize uint64) (compressedSize uint64, checksum uint32, err error) {

	//crc hasher
	crc := crc32.NewIEEE()

	//flate writer
	flateWriter, err := flate.NewWriter(writer.OutputFile, int(method))
	if err != nil {
		return 0, 0, err
	}

	multiWriter := io.MultiWriter(crc, flateWriter)

	ok := false

	defer func() {
		if !ok {
			//redo changes
			if err2 := flateWriter.Close(); err2 != nil {
				writer.Status = Failed
				err = fmt.Errorf("WRITER::failed to close compressor: %v", errors.Join(err, err2))
				return
			}

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

	n, err := io.Copy(multiWriter, file)
	if err != nil || n != int64(originalSize) {
		if err != nil {
			return 0, 0, err
		}

		return 0, 0, fmt.Errorf("WRITER::wrote %d bytes instead of %d bytes", n, originalSize)
	}

	if err := flateWriter.Close(); err != nil {
		return 0, 0, fmt.Errorf("WRITER::failed to close compressor: %v", err)
	}

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return 0, 0, err
	}

	compressedSize = uint64(outputInfo.Size()) - dataOffset
	checksum = crc.Sum32()
	ok = true

	return compressedSize, checksum, nil
}

func (writer *ArchiveWriter) AddFilesConcurrent(files []FileStruct, numWorkers int) error {

	if writer.Status != Active {
		return fmt.Errorf("WRITER::invalid ArchiveWriter status %d", writer.Status)
	}

	if numWorkers <= 0 {
		return fmt.Errorf("WRITER::worker count must be positive")
	}

	jobs := make([]JobCompress, 0, len(files))
	for _, file := range files {

		switch file.Method {
		case None, BestCompression, BestSpeed, Default, HuffmanOnly:
			break
		default:
			return fmt.Errorf("WRITER::incorrect compression method: %d", file.Method)
		}

		jobs = append(jobs, JobCompress{Path: file.Path, ArchivePath: file.ArchivePath, CompressionMethod: file.Method})
	}

	res := StartCompressionPool(jobs, numWorkers)

	if err := WriteArchive(writer, res); err != nil {
		return fmt.Errorf("WRITER::error writing to archive concurrently: %w", err)
	}

	return nil
}

func dirExists(path string) bool {

	info, err := os.Stat(path)
	if err != nil {

		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return false
	}

	return info.IsDir()
}

func (writer *ArchiveWriter) AddDirectory(dirPath string) error {

	if !dirExists(dirPath) {
		return fmt.Errorf("WRITER::error opening the directory")
	}

	var files []FileStruct

	if err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {

		if err != nil {
			return fmt.Errorf("error accessing path %q: %w", path, err)
		}

		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		root := filepath.Base(filepath.Clean(dirPath))
		relPath = filepath.ToSlash(filepath.Join(root, relPath))

		files = append(files, FileStruct{
			Path:        path,
			ArchivePath: relPath,
			Method:      Default})

		return nil
	}); err != nil {
		return err
	}

	return writer.AddFilesConcurrent(files, 4)
}

func (writer *ArchiveWriter) AddFile(fileInput FileStruct) error {

	if writer.Status != Active {
		return fmt.Errorf("WRITER::invalid ArchiveWriter status %d", writer.Status)
	}

	fileName := fileInput.ArchivePath
	if fileName == "" {
		fileName = filepath.Base(fileInput.Path)
	}
	fileName = filepath.ToSlash(fileName)

	if len(fileName) > 255 {
		return errors.New("WRITER::filename too long")
	}

	file, err := os.Open(fileInput.Path)
	if err != nil {
		return err
	}

	defer file.Close()

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITER::error reading output file stats: %v", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("WRITER::error reading file stats: %v", err)
	}

	dataOffset := uint64(outputInfo.Size())
	originalSize := uint64(fileInfo.Size())
	compressionMethod := fileInput.Method
	var checksum uint32
	var compressedSize uint64

	switch fileInput.Method {
	case None:
		//compress the data
		hasher := crc32.NewIEEE()

		//calculate checksum of original file & write to file
		multiWriter := io.MultiWriter(hasher, writer.OutputFile)

		n, err := io.Copy(multiWriter, file)
		if err != nil || n != int64(originalSize) {

			//redo changes
			rollbackErr := writer.OutputFile.Truncate(int64(dataOffset))
			if rollbackErr != nil {
				writer.Status = Failed
				return errors.Join(err, rollbackErr)
			}

			_, seekErr := writer.OutputFile.Seek(int64(dataOffset), io.SeekStart)
			if seekErr != nil {

				//memory truncated but file pointer not moved
				writer.Status = Failed
				return errors.Join(err, seekErr)
			}

			if err != nil {
				return err
			}

			return fmt.Errorf("WRITER::copied %d bytes, expected %d", n, originalSize)
		}

		checksum = hasher.Sum32()
		compressedSize = originalSize
	case Default, BestCompression, BestSpeed, HuffmanOnly:
		compressedSize, checksum, err = writer.writeCompressed(file, fileInput.Method, dataOffset, originalSize)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid compression method")
	}

	//make the fileEntry
	fileEntry := FileEntry{
		FilenameLength:    uint8(len(fileName)),
		Filename:          fileName,
		DataOffset:        dataOffset,
		OriginalSize:      originalSize,
		CompressedSize:    compressedSize,
		CompressionMethod: compressionMethod,
		Checksum:          checksum,
	}

	//append to pre-existing files
	writer.Files = append(writer.Files, fileEntry)

	return nil
}

func (writer *ArchiveWriter) Close() error {

	if writer.Status != Active {
		return fmt.Errorf("WRITER::invalid ArchiveWriter status %d", writer.Status)
	}

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITER::error reading output file stats: %v", err)
	}

	directoryOffset := uint64(outputInfo.Size())
	entryCount := uint32(len(writer.Files))

	//write the FileEntries to file
	for _, fileEntry := range writer.Files {

		buffer, err := SerializeFileEntry(&fileEntry)
		if err != nil {
			return err
		}

		n, err := writer.OutputFile.Write(buffer)
		if err != nil || n != len(buffer) {
			return fmt.Errorf("WRITER::wrote %d bytes instead of %d bytes. Error: %s", n, len(buffer), err)
		}
	}

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITER::error reading output file stats: %v", err)
	}

	directorySize := uint64(outputInfo.Size()) - directoryOffset

	//write the Footer to file
	archiveFooter := Footer{
		Signature:       fzFooterSignature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount,
	}

	buffer := SerializeFooter(&archiveFooter)

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("WRITER::error reading output file stats: %v", err)
	}

	//validate footer
	if directoryOffset+directorySize != uint64(outputInfo.Size()) {
		return errors.New("WRITER::invalid directory offset or size")
	}

	n, err := writer.OutputFile.Write(buffer[:])
	if err != nil || n != len(buffer) {
		return fmt.Errorf("WRITER::wrote %d bytes instead of %d. Error: %s", n, len(buffer), err)
	}

	if err := writer.OutputFile.Close(); err != nil {
		return err
	}

	writer.Status = Closed

	return nil
}
