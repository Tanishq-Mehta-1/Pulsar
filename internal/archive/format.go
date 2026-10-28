package archive

import (
	"encoding/binary"
	"errors"
)

type CompressionMethod uint8

const (
	None  CompressionMethod = 0
	Basic CompressionMethod = 1
)

const FileEntryLength int = 30

// FileEntryLength + variable bytes
type FileEntry struct {
	FilenameLength    uint8             //1 bytes
	Filename          string            //variable
	DataOffset        uint64            //8 bytes
	OriginalSize      uint64            //8 bytes
	CompressedSize    uint64            //8 bytes
	CompressionMethod CompressionMethod //1 bytes
	Checksum          uint32            //4 bytes
}

func SerializeFileEntry(entry *FileEntry) ([]byte, error) {

	if entry == nil {
		return nil, errors.New("empty FileEntry")
	}

	filenameLen := len(entry.Filename)
	if filenameLen > 255 {
		return nil, errors.New("filename too long")
	}

	//prechecks
	if len(entry.Filename) != int(entry.FilenameLength) {
		return nil, errors.New("filename length does not match")
	}

	buffer := make([]byte, FileEntryLength+filenameLen)

	copy(buffer[0:1], []byte{(byte(entry.FilenameLength))})
	copy(buffer[1:1+filenameLen], entry.Filename)
	binary.BigEndian.PutUint64(buffer[1+filenameLen:9+filenameLen], entry.DataOffset)
	binary.BigEndian.PutUint64(buffer[9+filenameLen:17+filenameLen], entry.OriginalSize)
	binary.BigEndian.PutUint64(buffer[17+filenameLen:25+filenameLen], entry.CompressedSize)
	copy(buffer[25+filenameLen:26+filenameLen], []byte{(byte(entry.CompressionMethod))})
	binary.BigEndian.PutUint32(buffer[26+filenameLen:], entry.Checksum)

	return buffer, nil
}

func DeserializeFileEntry(data []byte) (*FileEntry, int, error) {

	if len(data) < FileEntryLength {
		return nil, 0, errors.New("invalid size: file entry too small")
	}

	filenameLen := data[:1][0]

	//validate size
	if len(data) < int(filenameLen)+FileEntryLength {
		return nil, 0, errors.New("invalid size: file entry doesn't match size")
	}

	filename := string(data[1 : 1+filenameLen])
	dataOffset := binary.BigEndian.Uint64(data[1+filenameLen : 9+filenameLen])
	originalSize := binary.BigEndian.Uint64(data[9+filenameLen : 17+filenameLen])
	compressedSize := binary.BigEndian.Uint64(data[17+filenameLen : 25+filenameLen])
	compressionMethod := CompressionMethod(data[25+filenameLen : 26+filenameLen][0])
	checksum := binary.BigEndian.Uint32(data[26+filenameLen : FileEntryLength+int(filenameLen)])

	//validate compressionMethod
	switch compressionMethod {
	case None, Basic:
	default:
		return nil, 0, errors.New("invalid compression method")
	}

	return &FileEntry{
		FilenameLength:    filenameLen,
		Filename:          filename,
		DataOffset:        dataOffset,
		OriginalSize:      originalSize,
		CompressedSize:    compressedSize,
		CompressionMethod: compressionMethod,
		Checksum:          checksum,
	}, FileEntryLength + int(filenameLen), nil
}
