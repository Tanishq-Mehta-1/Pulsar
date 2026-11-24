package main

import (
	"fmt"
	"log"
	archive "pulsar/internal"
)

func write() {
	writer := archive.ArchiveWriterV2{}
	//4KB chunks
	if err := writer.CreateArchive("testArchive", "./testarchive", 4*1024); err != nil {
		log.Fatal(err)
	}

	if err := writer.AddDirectory("./testdata/text", 8); err != nil {
		log.Fatal(err)
	}

	// files := []archive.FileStruct{{Path: "/home/deo3d2y/Desktop/Dev/Pulsar/testdata/images/emoji.jpeg", Method: archive.Default}, {Path: "/home/deo3d2y/Desktop/Dev/Pulsar/internal/writerV1.go", Method: archive.Default}}

	// if err := writer.AddFilesConcurrent(files, 8); err != nil {
	// 	log.Fatal(err)
	// }

	if err := writer.Close(); err != nil {
		log.Fatal(err)
	}
}

func read() {
	reader := archive.ArchiveReader{}

	if err := reader.OpenArchive("./testarchive/testArchive.plsr"); err != nil {
		log.Fatal(err)
	}

	if err := reader.ExtractAllConcurrent("./testarchive/extracted", 8); err != nil {
		log.Fatal(err)
	}

	if err := reader.Close(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("wrote the file!\n")

}

func main() {
	write()

	read()
}
