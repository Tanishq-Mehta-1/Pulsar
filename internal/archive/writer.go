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
	Deleted WriterStatus = 3
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
		return fmt.Errorf("invalid ArchiveWriter status ", writer.Status)
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
	file.Write(byte_stream[:])

	writer.Status = Active

	return nil
}

func (writer *ArchiveWriter) AddFile(path string) error {

	if writer.Status != Active {
		return fmt.Errorf("invalid ArchiveWriter status ", writer.Status)
	}

	fileName := filepath.Base(path)

	if len(fileName) > 65535 {
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

	//calculate checksum of original file
	hasher := crc32.NewIEEE()

	if _, err = io.Copy(hasher, file); err != nil {
		return err
	}

	checksum := hasher.Sum32()

	//make the fileEntry
	fileEntry := FileEntry{
		FilenameLength:    uint16(len(fileName)),
		Filename:          fileName,
		DataOffset:        dataOffset,
		OriginalSize:      originalSize,
		CompressedSize:    originalSize,
		CompressionMethod: compressionMethod,
		Checksum:          checksum,
	}

	//append to pre-existing files
	writer.Files = append(writer.Files, fileEntry)

	//then we compress the file and write to the file

	return nil

}

func (writer *ArchiveWriter) Close() error {

	for _, fileEntry := range writer.Files {

		buffer, err := SerializeFileEntry(&fileEntry)
		if err != nil {
			return err
		}

		writer.OutputFile.Write(buffer)
	}

	writer.OutputFile.Close()
	writer.Status = Deleted

	return nil
}
