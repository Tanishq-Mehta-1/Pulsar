package archive

import (
	"encoding/binary"
	"errors"
)

const fzHeaderSignature = 0x11213141

// 12-byte archive header
type Header struct {
	Signature uint32
	Version   uint16
	Flags     uint16
	Reserved  uint32
}

// big Endian Representation
func SerializeHeader(header *Header) [12]byte {

	var byte_header [12]byte

	binary.BigEndian.PutUint32(byte_header[:4], header.Signature)
	binary.BigEndian.PutUint16(byte_header[4:6], header.Version)
	binary.BigEndian.PutUint16(byte_header[6:8], header.Flags)
	binary.BigEndian.PutUint32(byte_header[8:], header.Reserved)

	return byte_header
}

func DeserializeHeader(header_buffer [12]byte) (*Header, error) {

	signature := binary.BigEndian.Uint32(header_buffer[:4])
	version := binary.BigEndian.Uint16(header_buffer[4:6])
	flags := binary.BigEndian.Uint16(header_buffer[6:8])
	reserved := binary.BigEndian.Uint32(header_buffer[8:])

	//signature check
	if signature != fzHeaderSignature {
		return nil, errors.New("invalid file signature")
	}

	//version check
	switch version {
	case 1:
		break
	default:
		return nil, errors.New("invalid archive version")
	}

	//flag check
	if flags != 0 {
		return nil, errors.New("invalid flags")
	}

	//reserved check
	if reserved != 0 {
		return nil, errors.New("invalid reserved bytes")
	}

	return &Header{
		Signature: signature,
		Version:   version,
		Flags:     flags,
		Reserved:  reserved}, nil

}
