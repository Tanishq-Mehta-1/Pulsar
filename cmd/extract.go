package main

import (
	"fmt"
	archive "pulsar/internal"
)

func extract(path string, dst string, numWorkers int) error {

	reader := archive.ArchiveReader{}
	if err := reader.OpenArchive(path); err != nil {
		return err
	}

	defer reader.Close()

	if numWorkers < 0 {
		return fmt.Errorf("EXTRACT::invalid worker count")
	} else if numWorkers == 0 {
		numWorkers = archive.ResolveWorkers(numWorkers)
		fmt.Printf("Workers assigned: %d", numWorkers)
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
