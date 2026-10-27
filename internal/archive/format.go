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

// 31 + variable bytes
type FileEntry struct {
	FilenameLength    uint16            //2 bytes
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
	if filenameLen > 65535 {
		return nil, errors.New("filename too long")
	}

	//prechecks
	if len(entry.Filename) != int(entry.FilenameLength) {
		return nil, errors.New("filename length does not match")
	}

	buffer := make([]byte, 31+filenameLen)

	binary.BigEndian.PutUint16(buffer[0:2], entry.FilenameLength)
	copy(buffer[2:2+filenameLen], entry.Filename)
	binary.BigEndian.PutUint64(buffer[2+filenameLen:10+filenameLen], entry.DataOffset)
	binary.BigEndian.PutUint64(buffer[10+filenameLen:18+filenameLen], entry.OriginalSize)
	binary.BigEndian.PutUint64(buffer[18+filenameLen:26+filenameLen], entry.CompressedSize)
	copy(buffer[26+filenameLen:27+filenameLen], []byte{(byte(entry.CompressionMethod))})
	binary.BigEndian.PutUint32(buffer[27+filenameLen:], entry.Checksum)

	return buffer, nil
}

func DeserializeFileEntry(data []byte) (*FileEntry, int, error) {

	if len(data) < 2 {
		return nil, 0, errors.New("invalid size: file entry too small")
	}

	filenameLen := binary.BigEndian.Uint16(data[:2])

	//validate size
	if len(data) < int(filenameLen)+31 {
		return nil, 0, errors.New("invalid size: file entry doesn't match size")
	}

	filename := string(data[2 : 2+filenameLen])
	dataOffset := binary.BigEndian.Uint64(data[2+filenameLen : 10+filenameLen])
	originalSize := binary.BigEndian.Uint64(data[10+filenameLen : 18+filenameLen])
	compressedSize := binary.BigEndian.Uint64(data[18+filenameLen : 26+filenameLen])
	compressionMethod := CompressionMethod(data[26+filenameLen : 27+filenameLen][0])
	checksum := binary.BigEndian.Uint32(data[27+filenameLen : 31+filenameLen])

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
	}, int(31 + filenameLen), nil
}
