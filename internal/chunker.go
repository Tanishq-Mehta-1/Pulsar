package archive

type ChunkJob struct {
	FileID     uint32
	ChunkIndex uint32
	Data       []byte
	Method     CompressionMethod
}

type ChunkResult struct {
	FileID            uint32
	ChunkIndex        uint32
	OriginalSize      uint64
	CompressedSize    uint64
	CompressionMethod CompressionMethod
	Checksum          uint32
	Data              []byte
}
