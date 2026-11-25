package archive

import (
	"encoding/binary"
	"fmt"
)

const fzFooterSignature = 0x51617181
const FooterSizeV1 int = 24
const FooterSizeV2 int = 28

// 24-byte archive
type FooterV1 struct {
	Signature       uint32
	DirectoryOffset uint64
	DirectorySize   uint64
	EntryCount      uint32
}

// 28-byte archive
type FooterV2 struct {
	Signature       uint32
	DirectoryOffset uint64
	DirectorySize   uint64
	EntryCount      uint32
	ChunkCount      uint32
}

// big Endian Representation
func SerializeFooterV1(footer *FooterV1) [FooterSizeV1]byte {

	var byte_footer [FooterSizeV1]byte

	binary.BigEndian.PutUint32(byte_footer[:4], footer.Signature)
	binary.BigEndian.PutUint64(byte_footer[4:12], footer.DirectoryOffset)
	binary.BigEndian.PutUint64(byte_footer[12:20], footer.DirectorySize)
	binary.BigEndian.PutUint32(byte_footer[20:], footer.EntryCount)

	return byte_footer
}

func DeserializeFooterV1(footer_buffer [FooterSizeV1]byte) (*FooterV1, error) {

	signature := binary.BigEndian.Uint32(footer_buffer[:4])
	directoryOffset := binary.BigEndian.Uint64(footer_buffer[4:12])
	directorySize := binary.BigEndian.Uint64(footer_buffer[12:20])
	entryCount := binary.BigEndian.Uint32(footer_buffer[20:])

	//signature check
	if signature != fzFooterSignature {
		return nil, fmt.Errorf("FOOTER::invalid file footer signature, read %x instead of %x", signature, fzFooterSignature)
	}

	return &FooterV1{
		Signature:       signature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount,
	}, nil
}

func SerializeFooterV2(footer *FooterV2) [FooterSizeV2]byte {

	var byte_footer [FooterSizeV2]byte

	binary.BigEndian.PutUint32(byte_footer[:4], footer.Signature)
	binary.BigEndian.PutUint64(byte_footer[4:12], footer.DirectoryOffset)
	binary.BigEndian.PutUint64(byte_footer[12:20], footer.DirectorySize)
	binary.BigEndian.PutUint32(byte_footer[20:24], footer.EntryCount)
	binary.BigEndian.PutUint32(byte_footer[24:], footer.ChunkCount)

	return byte_footer
}

func DeserializeFooterV2(footer_buffer [FooterSizeV2]byte) (*FooterV2, error) {

	signature := binary.BigEndian.Uint32(footer_buffer[:4])
	directoryOffset := binary.BigEndian.Uint64(footer_buffer[4:12])
	directorySize := binary.BigEndian.Uint64(footer_buffer[12:20])
	entryCount := binary.BigEndian.Uint32(footer_buffer[20:24])
	chunkCount := binary.BigEndian.Uint32(footer_buffer[24:])

	//signature check
	if signature != fzFooterSignature {
		return nil, fmt.Errorf("FOOTER::invalid file footer signature, read %x instead of %x", signature, fzFooterSignature)
	}

	return &FooterV2{
		Signature:       signature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount,
		ChunkCount:      chunkCount,
	}, nil
}
