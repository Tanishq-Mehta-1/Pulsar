package main

import (
	"fmt"
	"log"
	"os"
	"pulsar/internal/archive"
	"strconv"
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
}

func test_reader() {
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

func handle_invalid() {
	fmt.Println("Invalid use of command")

}

func main() {
	// programName := os.Args[0]

	if len(os.Args) < 2 {
		handle_invalid()
		return
	}

	arguments := os.Args[1:]
	if len(arguments) < 2 {
		handle_invalid()
		return
	}

	switch arguments[0] {
	case "list":
		archivePath := arguments[1]
		if err := list(archivePath); err != nil {
			log.Fatal(fmt.Errorf("PLSR::LIST::%w", err))
		}

	case "extract":
		archivePath := arguments[1]

		outputDir := ""
		numWorkers := -1

		if len(arguments) > 2 {

			for i, n := 2, len(arguments); i < n; i++ {
				arg := arguments[i]

				if arg == "-o" && i+1 < n {
					outputDir = arguments[i+1]
					i++
				} else if arg == "--workers" && i+1 < n {
					var err error
					numWorkers, err = strconv.Atoi(arguments[i+1])
					if err != nil {
						log.Fatal(fmt.Errorf("PLSR::EXTRACT::%w", err))
					}
					i++
				} else {
					handle_invalid()
					return
				}
			}
		}

		if err := extract(archivePath, outputDir, numWorkers); err != nil {
			log.Fatal(fmt.Errorf("PLSR::EXTRACT::%w", err))
		}
	default:
		handle_invalid()
	}

	// test_writer()
	// test_reader()
}
