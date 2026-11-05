package main

import (
	"fmt"
	"pulsar/internal/archive"
)

func extract(path string, dst string, numWorkers int) error {

	reader := archive.ArchiveReader{}
	if err := reader.OpenArchive(path); err != nil {
		return err
	}

	defer reader.Close()

	if numWorkers == -1 {
		numWorkers = 8
	} else if numWorkers <= 0 {
		return fmt.Errorf("EXTRACT::worker count must be positive")
	}

	if err := reader.ExtractAllConcurrent(dst, numWorkers); err != nil {
		return err
	}

	if dst != "" {
		fmt.Printf("Extracted to %s\n", dst)
	} else {
		fmt.Printf("Extracted locally\n")
	}

	return nil
}
