package main

import (
	"fmt"
	"log"
	"os"
	"pulsar/internal/archive"
)

func test_writer() {
	writer := archive.ArchiveWriter{}

	if err := writer.CreateArchive("archive", "output"); err != nil {
		log.Fatalf("WRITER::%s", err)
	}

	// files := []archive.FileStruct{
	// 	{Path: "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/a.txt",
	// 		Method: archive.Default},
	// 	{Path: "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/b.txt",
	// 		Method: archive.Default},
	// 	{Path: "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/big.txt",
	// 		Method: archive.Default},
	// 	{Path: "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/emoji.jpeg",
	// 		Method: archive.Default}}

	// if err := writer.AddFilesConcurrent(files, 4); err != nil {
	// log.Fatalf("WRITER::%s", err)
	// }

	if err := writer.AddDirectory("/home/deo3d2y/Desktop/Dev/Pulsar/testdata"); err != nil {
		log.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		log.Fatalf("WRITER::%s", err)
	}

	reader := archive.ArchiveReader{}
	err := reader.OpenArchive("output/archive.fz")
	if err != nil {
		log.Fatalf("Archive error: %s", err)
	}

	reader.ExtractAllConcurrent("extract", 2)

	if err = reader.Close(); err != nil {
		log.Fatalf("Archive::%s", err)
	}
}

func test_goroutines() {

	res := archive.CompressFile(archive.JobCompress{
		Path:              "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/big.txt",
		CompressionMethod: -2})
	fmt.Print(res)

	os.Remove(res.TempPath)
}

func main() {
	test_writer()
}
