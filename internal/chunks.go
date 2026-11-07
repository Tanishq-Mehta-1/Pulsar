package archive

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const ChunkEntryLength int = 37

type ChunkEntry struct {
	FileID            uint32            //4 bytes
	ChunkIndex        uint32            //4 bytes
	DataOffset        uint64            //8 bytes
	OriginalSize      uint64            //8 bytes
	CompressedSize    uint64            //8 bytes
	CompressionMethod CompressionMethod //1 byte
	Checksum          uint32            //4 bytes
}

const FileEntryChunkLength int = 17

type FileEntryChunk struct {
	FilenameLength uint8  //1 bytes
	Filename       string //variable
	FileID         uint32 //4 bytes
	OriginalSize   uint64 //8 bytes
	ChunkCount     uint32 //4 bytes
}

func SerializeFileEntryChunk(entry *FileEntryChunk) ([]byte, error) {

	if entry == nil {
		return nil, errors.New("CHUNK::empty FileEntryChunk")
	}

	filenameLen := len(entry.Filename)
	if filenameLen > 255 {
		return nil, errors.New("CHUNK::filename too long")
	}

	//prechecks
	if len(entry.Filename) != int(entry.FilenameLength) {
		return nil, errors.New("CHUNK::filename length does not match")
	}

	buffer := make([]byte, FileEntryChunkLength+filenameLen)

	copy(buffer[0:1], []byte{(byte(entry.FilenameLength))})
	copy(buffer[1:1+filenameLen], entry.Filename)
	binary.BigEndian.PutUint32(buffer[1+filenameLen:5+filenameLen], entry.FileID)
	binary.BigEndian.PutUint64(buffer[5+filenameLen:13+filenameLen], entry.OriginalSize)
	binary.BigEndian.PutUint32(buffer[13+filenameLen:17+filenameLen], entry.ChunkCount)

	return buffer, nil
}

func DeserializeFileEntryChunk(data []byte) (*FileEntryChunk, int, error) {

	if len(data) < FileEntryChunkLength {
		return nil, 0, errors.New("CHUNK::file entry too small")
	}

	filenameLen := int(data[0])

	if len(data) < FileEntryChunkLength+filenameLen {
		return nil, 0, errors.New("CHUNK::file entry too small")
	}

	//validate size
	if len(data) < int(filenameLen)+FileEntryChunkLength {
		return nil, 0, errors.New("CHUNK::file entry doesn't match size")
	}

	filename := string(data[1 : 1+filenameLen])
	fileID := binary.BigEndian.Uint32(data[1+filenameLen : 5+filenameLen])
	originalSize := binary.BigEndian.Uint64(data[5+filenameLen : 13+filenameLen])
	chunkCount := binary.BigEndian.Uint32(data[13+filenameLen : FileEntryChunkLength+filenameLen])

	return &FileEntryChunk{
		FilenameLength: byte(filenameLen),
		Filename:       filename,
		FileID:         fileID,
		OriginalSize:   originalSize,
		ChunkCount:     chunkCount,
	}, FileEntryChunkLength + int(filenameLen), nil
}

func SerializeChunkEntry(entry *ChunkEntry) ([]byte, error) {

	if entry == nil {
		return nil, errors.New("CHUNK::empty ChunkEntry")
	}

	buffer := make([]byte, ChunkEntryLength)

	binary.BigEndian.PutUint32(buffer[0:4], entry.FileID)
	binary.BigEndian.PutUint32(buffer[4:8], entry.ChunkIndex)
	binary.BigEndian.PutUint64(buffer[8:16], entry.DataOffset)
	binary.BigEndian.PutUint64(buffer[16:24], entry.OriginalSize)
	binary.BigEndian.PutUint64(buffer[24:32], entry.CompressedSize)
	copy(buffer[32:33], []byte{(byte(entry.CompressionMethod))})
	binary.BigEndian.PutUint32(buffer[33:37], entry.Checksum)

	return buffer, nil
}

func DeserializeChunkEntry(data []byte) (*ChunkEntry, int, error) {
	if len(data) < ChunkEntryLength {
		return nil, 0, fmt.Errorf("CHUNK::mismatching size. Given data: %d, ChunkEntry: %d", len(data), ChunkEntryLength)
	}

	fileID := binary.BigEndian.Uint32(data[0:4])
	chunkIndex := binary.BigEndian.Uint32(data[4:8])
	dataOffset := binary.BigEndian.Uint64(data[8:16])
	originalSize := binary.BigEndian.Uint64(data[16:24])
	compressedSize := binary.BigEndian.Uint64(data[24:32])
	compressionMethod := CompressionMethod(data[32:33][0])
	checksum := binary.BigEndian.Uint32(data[33:ChunkEntryLength])

	//validate compressionMethod
	switch compressionMethod {
	case None, Default, HuffmanOnly, BestSpeed, BestCompression:
	default:
		return nil, 0, errors.New("CHUNK::invalid compression method")
	}

	return &ChunkEntry{
		FileID:            fileID,
		ChunkIndex:        chunkIndex,
		DataOffset:        dataOffset,
		OriginalSize:      originalSize,
		CompressedSize:    compressedSize,
		CompressionMethod: compressionMethod,
		Checksum:          checksum,
	}, ChunkEntryLength, nil
}
