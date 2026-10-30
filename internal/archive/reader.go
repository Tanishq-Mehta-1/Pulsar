package archive

import (
	"compress/flate"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
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

		switch fileEntry.CompressionMethod {
		case None, BestCompression, BestSpeed, HuffmanOnly, Default:
			break
		default:
			return fmt.Errorf("READER::incorrect compression method")
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

func (reader *ArchiveReader) ExtractFile(fileName string, dst string) (err error) {

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

	dist, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer dist.Close()

	//extract file and put into dist
	success := false

	defer func() {
		if !success {
			if rmErr := os.Remove(dist.Name()); err != nil {
				err = errors.Join(err, fmt.Errorf("READER::error deleting file: %s after failed extraction", dist.Name()), rmErr)
				return
			}

			return
		}
	}()

	switch requiredFileEntry.CompressionMethod {
	case None:

		hasher := crc32.NewIEEE()
		multiWriter := io.MultiWriter(hasher, dist)

		sectionReader := io.NewSectionReader(
			reader.OutputFile,
			int64(requiredFileEntry.DataOffset),
			int64(requiredFileEntry.OriginalSize))

		n, err := io.Copy(multiWriter, sectionReader)
		if n != int64(requiredFileEntry.OriginalSize) || err != nil {
			if err != nil {
				return err
			}

			err = fmt.Errorf("READER::wrote %d bytes instead of %d", n, requiredFileEntry.OriginalSize)
			return err
		}

		checksum := hasher.Sum32()
		if checksum != requiredFileEntry.Checksum {
			err = fmt.Errorf("READER::checksum invalid. Calculated: %d, Actual: %d", checksum, requiredFileEntry.Checksum)
			return err
		}

		break

	case Default, BestSpeed, BestCompression, HuffmanOnly:

		if err = reader.extractCompressed(requiredFileEntry, dist); err != nil {
			return err
		}

		break
	default:
		return errors.New("READER::unsupported compression method")
	}

	success = true
	return nil
}

func (reader *ArchiveReader) extractCompressed(entry FileEntry, dst *os.File) (err error) {

	sectionReader := io.NewSectionReader(reader.OutputFile, int64(entry.DataOffset), int64(entry.CompressedSize))
	flateReader := flate.NewReader(sectionReader)

	defer flateReader.Close()

	hasher := crc32.NewIEEE()

	multiWriter := io.MultiWriter(hasher, dst)
	n, err := io.Copy(multiWriter, flateReader)
	if err != nil || n != int64(entry.OriginalSize) {
		if err != nil {
			return
		}

		err = fmt.Errorf("READER::wrote %d bytes instead of %d", n, entry.OriginalSize)
		return
	}

	checksum := hasher.Sum32()
	if checksum != entry.Checksum {
		err = fmt.Errorf("READER::checksum invalid. Calculated: %d, Actual: %d", checksum, entry.Checksum)
		return
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
