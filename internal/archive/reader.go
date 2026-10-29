package archive

import (
	"errors"
	"fmt"
	"os"
)

type ArchiveReader struct {
	OutputFile    *os.File
	ArchiveFooter Footer
	Files         []FileEntry
	Status        ObjStatus
}

func (reader *ArchiveReader) OpenArchive(path string) error {

	if reader.Status != Created {
		return fmt.Errorf("invalid ArchiveWriter status %d", reader.Status)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	outputFileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	if outputFileInfo.Size() < int64(HeaderSize+FooterSize) {
		return fmt.Errorf("selected archive is too small")
	}

	reader.OutputFile = file

	// //read header
	// headerBuffer := make([]byte, HeaderSize)

	// n, err := file.ReadAt(headerBuffer, 0)
	// if err != nil || n != HeaderSize {
	// 	if err != nil {
	// 		return err
	// 	}

	// 	return fmt.Errorf("read %d bytes instead of %d", n, HeaderSize)
	// }

	// archiveHeader, err := DeserializeHeader([12]byte(headerBuffer))
	// if err != nil {
	// 	return err
	// }

	//read footer
	footerBuffer := make([]byte, FooterSize)
	n, err := file.ReadAt(footerBuffer, outputFileInfo.Size()-int64(FooterSize))
	if err != nil || n != FooterSize {
		if err != nil {
			return err
		}

		return fmt.Errorf("read %d bytes instead of %d", n, FooterSize)
	}

	archiveFooter, err := DeserializeFooter([24]byte(footerBuffer))
	if err != nil {
		return err
	}

	//validate footer
	if archiveFooter.DirectoryOffset+archiveFooter.DirectorySize+uint64(FooterSize) != uint64(outputFileInfo.Size()) {
		return fmt.Errorf("footer directory offset and size don't match the archive's size")
	}

	//read the directory
	directoryData := make([]byte, archiveFooter.DirectorySize)
	n, err = file.ReadAt(directoryData, int64(archiveFooter.DirectoryOffset))
	if err != nil || n != int(archiveFooter.DirectorySize) {
		if err != nil {
			return err
		}

		return fmt.Errorf("read %d bytes instead of %d", n, archiveFooter.DirectorySize)
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

		reader.Files = append(reader.Files, *fileEntry)
		position += bytesRead
		filesRead++
	}

	if position != len(directoryData) {
		//undo changes
		reader.Files = nil
		return errors.New("error reading central directory")
	}

	if filesRead != int(archiveFooter.EntryCount) {
		//undo changes
		reader.Files = nil
		return fmt.Errorf("read %d files instead of %d", filesRead, archiveFooter.EntryCount)
	}

	reader.Status = Active

	return nil
}
