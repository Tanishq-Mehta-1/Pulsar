package main

import (
	"log"
	"pulsar/internal/archive"
)

func main() {
	// writer := archive.ArchiveWriter{}

	// if err := writer.CreateArchive("archive"); err != nil {
	// 	log.Fatalf("WRITER::%s", err)
	// }
	// if err := writer.AddFile("/home/deo3d2y/Desktop/Dev/Pulsar/testdata/a.txt"); err != nil {
	// 	log.Fatalf("WRITER::%s", err)
	// }
	// if err := writer.AddFile("/home/deo3d2y/Desktop/Dev/Pulsar/testdata/b.txt"); err != nil {
	// 	log.Fatalf("WRITER::%s", err)
	// }
	// if err := writer.Close(); err != nil {
	// 	log.Fatalf("WRITER::%s", err)
	// }

	reader := archive.ArchiveReader{}
	err := reader.OpenArchive("archive.fz")
	if err != nil {
		log.Fatalf("Archive error: %s", err)
	}
	if err = reader.ExtractFile("a.txt", "a.txt"); err != nil {
		log.Fatalf("Error reading a.txt: %s", err)

	}
	if err = reader.ExtractFile("b.txt", "b.txt"); err != nil {
		log.Fatalf("Error reading b.txt: %s", err)
	}
}
