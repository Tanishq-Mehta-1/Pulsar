package archive

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
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

func checkFilename(fileName string) bool {
	if len(fileName) > 255 || strings.Contains(fileName, "/") {
		return false
	}

	return true
}

func (writer *ArchiveWriter) CreateArchive(fileName string) error {

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
				err = errors.Join(err, rollbackErr)
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

func (writer *ArchiveWriter) AddFile(path string, method CompressionMethod) error {

	if writer.Status != Active {
		return fmt.Errorf("WRITER::invalid ArchiveWriter status %d", writer.Status)
	}

	fileName := filepath.Base(path)

	if len(fileName) > 255 {
		return errors.New("WRITER::filename too long")
	}

	file, err := os.Open(path)
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
	compressionMethod := method
	var checksum uint32
	var compressedSize uint64

	switch method {
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
		break
	case Default, BestCompression, BestSpeed, HuffmanOnly:
		compressedSize, checksum, err = writer.writeCompressed(file, method, dataOffset, originalSize)
		if err != nil {
			return err
		}
		break
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
