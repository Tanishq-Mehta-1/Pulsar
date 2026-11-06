package archive

import (
	"encoding/binary"
	"fmt"
)

const fzFooterSignature = 0x51617181
const FooterSize int = 24

// 24-byte archive
type Footer struct {
	Signature       uint32
	DirectoryOffset uint64
	DirectorySize   uint64
	EntryCount      uint32
}

// big Endian Representation
func SerializeFooter(footer *Footer) [FooterSize]byte {

	var byte_footer [FooterSize]byte

	binary.BigEndian.PutUint32(byte_footer[:4], footer.Signature)
	binary.BigEndian.PutUint64(byte_footer[4:12], footer.DirectoryOffset)
	binary.BigEndian.PutUint64(byte_footer[12:20], footer.DirectorySize)
	binary.BigEndian.PutUint32(byte_footer[20:], footer.EntryCount)

	return byte_footer
}

func DeserializeFooter(footer_buffer [FooterSize]byte) (*Footer, error) {

	signature := binary.BigEndian.Uint32(footer_buffer[:4])
	directoryOffset := binary.BigEndian.Uint64(footer_buffer[4:12])
	directorySize := binary.BigEndian.Uint64(footer_buffer[12:20])
	entryCount := binary.BigEndian.Uint32(footer_buffer[20:])

	//signature check
	if signature != fzFooterSignature {
		return nil, fmt.Errorf("FOOTER::invalid file footer signature, read %x instead of %x", signature, fzFooterSignature)
	}

	return &Footer{
		Signature:       signature,
		DirectoryOffset: directoryOffset,
		DirectorySize:   directorySize,
		EntryCount:      entryCount}, nil

}
