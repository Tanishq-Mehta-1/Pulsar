package archive

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var extension string = ".fz"

type WriterStatus uint8

const (
	Created WriterStatus = 0
	Active  WriterStatus = 2
	Closed  WriterStatus = 3
)

type ArchiveWriter struct {
	OutputFile *os.File
	Files      []FileEntry
	Status     WriterStatus
}

func checkFilename(fileName string) bool {
	if len(fileName) > 255 || strings.Contains(fileName, "/") {
		return false
	}

	return true
}

func (writer *ArchiveWriter) CreateArchive(fileName string) error {

	if writer.Status != Created {
		return fmt.Errorf("invalid ArchiveWriter status %d", writer.Status)
	}

	if !checkFilename(fileName) {
		return errors.New("invalid filename")
	}

	if fileName == "" {
		fileName = "output"
	}

	fileName += extension

	file, err := os.Create(fileName)
	if err != nil {
		return err
	}

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
		if err2 := writer.OutputFile.Close(); err2 != nil {
			return errors.Join(err2, fmt.Errorf("wrote %d bytes instead of %d. Error: %s", n, len(byte_stream), err))
		}
		writer.OutputFile = nil
		return fmt.Errorf("wrote %d bytes instead of %d. Error: %s", n, len(byte_stream), err)
	}

	writer.Status = Active

	return nil
}

func (writer *ArchiveWriter) AddFile(path string) error {

	if writer.Status != Active {
		return fmt.Errorf("invalid ArchiveWriter status %d", writer.Status)
	}

	fileName := filepath.Base(path)

	if len(fileName) > 255 {
		return errors.New("filename too long")
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	defer file.Close()

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("error reading output file stats: %v", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("error reading file stats: %v", err)
	}

	dataOffset := uint64(outputInfo.Size())
	originalSize := uint64(fileInfo.Size())
	compressionMethod := None

	//calculate checksum of original file & write to file
	hasher := crc32.NewIEEE()

	multiWriter := io.MultiWriter(hasher, writer.OutputFile)

	if n, err := io.Copy(multiWriter, file); err != nil || n != int64(originalSize) {

		//redo changes
		if err2 := writer.OutputFile.Truncate(int64(dataOffset)); err != nil {
			return errors.Join(err2, err)
		}

		if _, err2 := writer.OutputFile.Seek(int64(dataOffset), 0); err != nil {
			return errors.Join(err2, err)
		}

		if n != int64(originalSize) {
			return errors.Join(fmt.Errorf("%d bytes written instead of %d", n, originalSize), err)
		}

		return err
	}

	checksum := hasher.Sum32()

	//make the fileEntry
	fileEntry := FileEntry{
		FilenameLength:    uint8(len(fileName)),
		Filename:          fileName,
		DataOffset:        dataOffset,
		OriginalSize:      originalSize,
		CompressedSize:    originalSize,
		CompressionMethod: compressionMethod,
		Checksum:          checksum,
	}

	//append to pre-existing files
	writer.Files = append(writer.Files, fileEntry)

	return nil
}

func (writer *ArchiveWriter) Close() error {

	if writer.Status != Active {
		return fmt.Errorf("invalid ArchiveWriter status %d", writer.Status)
	}

	outputInfo, err := writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("error reading output file stats: %v", err)
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
			return fmt.Errorf("wrote %d bytes instead of %d bytes. Error: %s", n, len(buffer), err)
		}
	}

	outputInfo, err = writer.OutputFile.Stat()
	if err != nil {
		return fmt.Errorf("error reading output file stats: %v", err)
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
		return fmt.Errorf("error reading output file stats: %v", err)
	}

	//validate footer
	if directoryOffset != uint64(outputInfo.Size())-uint64(FooterSize) {
		return errors.New("invalide directory offset in the footer")
	}

	if directorySize != uint64(outputInfo.Size())-uint64(FooterSize)-directoryOffset {
		return errors.New("invalid directory size in the footer")
	}

	n, err := writer.OutputFile.Write(buffer[:])
	if err != nil || n != len(buffer) {
		return fmt.Errorf("wrote %d bytes instead of %d. Error: %s", n, len(buffer), err)
	}

	if err := writer.OutputFile.Close(); err != nil {
		return err
	}

	writer.Status = Closed

	return nil
}
