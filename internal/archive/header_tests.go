package archive

import "testing"

func TestSerializeHeader(t *testing.T) {

	//test signature
	testHeader := &Header{Signature: 12, Version: 1, Flags: 0, Reserved: 0}
	got := SerializeHeader(testHeader)
}
