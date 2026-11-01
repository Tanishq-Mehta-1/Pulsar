package archive

import (
	"bytes"
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

func Consumer(job Job) {

	res := CompressFile(job)

	originalData, err := os.ReadFile(job.Path)
	if err != nil {
		panic(err)
	}

	var compressed bytes.Buffer
	n, err := io.Copy(&compressed, res.Reader)
	if err != nil {
		panic(err)
	}
	metadata := <-res.Metadata

	if metadata.Err != nil {
		panic(metadata.Err)
	}

	if metadata.CompressedSize != uint64(n) {
		panic(fmt.Errorf("incorrect compressed size"))
	}

	flateReader := flate.NewReader(&compressed)
	defer flateReader.Close()

	var decompressed bytes.Buffer

	_, err = io.Copy(&decompressed, flateReader)
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(originalData, decompressed.Bytes()) {
		panic(fmt.Errorf("decompressed data does not match original"))
	}

	checksum := crc32.ChecksumIEEE(decompressed.Bytes())
	if checksum != metadata.Checksum {
		panic(fmt.Errorf("incorrect checksum"))
	}

	fmt.Println("Success!")
}
