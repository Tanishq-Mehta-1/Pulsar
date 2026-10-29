package archive

import (
	"errors"
	"fmt"
	"hash/crc32"
	"os"
)

type ArchiveReader struct {
	OutputFile *os.File
	Files      []FileEntry
	Status     ObjStatus
}

func (reader *ArchiveReader) OpenArchive(path string) error {

	if reader.Status != Created {
		return fmt.Errorf("READER::invalid ArchiveWriter status %d", reader.Status)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	ok := false

	defer func() {
		if !ok {
			file.Close()
			reader.Files = nil
		}
	}()

	outputFileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	if outputFileInfo.Size() < int64(HeaderSize+FooterSize) {
		return fmt.Errorf("READER::selected archive is too small")
	}

	//read header
	headerBuffer := make([]byte, HeaderSize)

	n, err := file.ReadAt(headerBuffer, 0)
	if err != nil || n != HeaderSize {
		if err != nil {
			return err
		}

		return fmt.Errorf("READER::read %d bytes instead of %d", n, HeaderSize)
	}

	_, err = DeserializeHeader([12]byte(headerBuffer))
	if err != nil {
		return err
	}

	//read footer
	footerBuffer := make([]byte, FooterSize)
	n, err = file.ReadAt(footerBuffer, outputFileInfo.Size()-int64(FooterSize))
	if err != nil || n != FooterSize {
		if err != nil {
			return err
		}

		return fmt.Errorf("READER::read %d bytes instead of %d", n, FooterSize)
	}

	archiveFooter, err := DeserializeFooter([24]byte(footerBuffer))
	if err != nil {
		return err
	}

	//validate footer
	size := uint64(outputFileInfo.Size())
	if archiveFooter.DirectoryOffset < uint64(HeaderSize) ||
		archiveFooter.DirectoryOffset > size || archiveFooter.DirectorySize > size ||
		archiveFooter.DirectoryOffset+archiveFooter.DirectorySize+uint64(FooterSize) != size {
		return fmt.Errorf("READER::footer directory offset and size don't match the archive's size")

	}

	// if archiveFooter.DirectoryOffset+archiveFooter.DirectorySize+uint64(FooterSize) != uint64(outputFileInfo.Size()) {
	// 	return fmt.Errorf("READER::footer directory offset and size don't match the archive's size")
	// }

	//read the directory
	directoryData := make([]byte, archiveFooter.DirectorySize)
	n, err = file.ReadAt(directoryData, int64(archiveFooter.DirectoryOffset))
	if err != nil || n != int(archiveFooter.DirectorySize) {
		if err != nil {
			return err
		}

		return fmt.Errorf("READER::read %d bytes instead of %d", n, archiveFooter.DirectorySize)
	}

	//extract file entries
	position := 0
	filesRead := 0

	for position < len(directoryData) {
		fileEntry, bytesRead, err := DeserializeFileEntry(directoryData[position:])
		if err != nil {

			//undo changes
			reader.Files = nil
			return err
		}

		end := fileEntry.DataOffset + fileEntry.CompressedSize
		if fileEntry.DataOffset < uint64(HeaderSize) || end < fileEntry.DataOffset || end > archiveFooter.DirectoryOffset {
			return fmt.Errorf("READER::incorect file entry data offset")

		}
		if fileEntry.CompressionMethod == None && fileEntry.OriginalSize != fileEntry.CompressedSize {
			return fmt.Errorf("READER::incorrect compression for Method: NONE")
		}

		reader.Files = append(reader.Files, *fileEntry)
		position += bytesRead
		filesRead++
	}

	if position != len(directoryData) {
		//undo changes
		reader.Files = nil
		return errors.New("READER::error reading central directory")
	}

	if filesRead != int(archiveFooter.EntryCount) {
		//undo changes
		reader.Files = nil
		return fmt.Errorf("READER::read %d files instead of %d", filesRead, archiveFooter.EntryCount)
	}

	reader.OutputFile = file
	reader.Status = Active
	ok = true

	return nil
}

func (reader *ArchiveReader) ExtractFile(fileName string, dst string) error {

	if reader.Status != Active {
		return fmt.Errorf("READER::invalid ArchiveWriter status %d", reader.Status)
	}

	if len(fileName) > 255 {
		return errors.New("READER::filename is too long")
	}

	var requiredFileEntry FileEntry
	found := false
	for _, fileEntry := range reader.Files {
		if fileEntry.Filename == fileName {
			requiredFileEntry, found = fileEntry, true
			break
		}
	}

	if !found {
		return fmt.Errorf("READER::no file with name: %s found in the archive", fileName)
	}

	fileBuffer := make([]byte, requiredFileEntry.CompressedSize)
	n, err := reader.OutputFile.ReadAt(fileBuffer, int64(requiredFileEntry.DataOffset))
	if err != nil || n != int(requiredFileEntry.CompressedSize) {

		if err != nil {
			return err
		}

		return fmt.Errorf("READER::read %d bytes instead of %d", n, requiredFileEntry.CompressedSize)
	}

	//decompress later, for now nothing

	//check checksum and write to the destination
	table := crc32.MakeTable(crc32.IEEE)
	checksum := crc32.Checksum(fileBuffer, table)

	if checksum != requiredFileEntry.Checksum {
		return errors.New("READER::checksum failed!")
	}

	err = os.WriteFile(dst, fileBuffer, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (reader *ArchiveReader) Close() error {

	err := reader.OutputFile.Close()
	if err != nil {
		return fmt.Errorf("READER::%s", err)
	}
	reader.Status = Closed

	return nil
}
