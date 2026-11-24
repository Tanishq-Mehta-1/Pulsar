package main

import (
	"errors"
	"fmt"
	"os"
	archive "pulsar/internal"
)

const KB = 1024

func processArg(
	path string,
	files,
	dirs []string) ([]string, []string, error) {

	info, err := os.Stat(path)
	if err != nil {
		return files, dirs, err
	}

	if info.IsDir() {
		dirs = append(dirs, path)
	} else if info.Mode().IsRegular() {
		files = append(files, path)
	} else {
		return files, dirs, fmt.Errorf("unsupported input type: %s", path)
	}

	return files, dirs, nil
}

func createV1(
	path string,
	files []string,
	dirs []string,
	outputDir string,
	numWorkers int) (err error) {
	writer := archive.ArchiveWriterV1{}
	if err := writer.CreateArchive(path, outputDir); err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

	// f, err := os.Create("compress.prof")
	// if err != nil {
	// 	return err
	// }

	// if err := pprof.StartCPUProfile(f); err != nil {
	// 	f.Close()
	// 	return err
	// }

	defer func() {
		if err != nil {
			writer.OutputFile.Close()
			remErr := os.Remove(writer.OutputFile.Name())
			err = errors.Join(err, remErr)
		}
	}()

	fileEntries := make([]archive.FileStruct, len(files))

	for i, file := range files {
		fileEntries[i] = archive.FileStruct{Path: file, Method: archive.Default}
	}

	if err := writer.AddFilesConcurrent(fileEntries, numWorkers); err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

	for _, dir := range dirs {
		if err := writer.AddDirectory(dir, numWorkers); err != nil {
			return fmt.Errorf("CREATE::%w", err)
		}
	}

	// if err := writer.AddFilesConcurrent(fileEntries, numWorkers); err != nil {
	// 	pprof.StopCPUProfile()
	// 	f.Close()
	// 	return fmt.Errorf("CREATE::%w", err)
	// }

	// pprof.StopCPUProfile()
	// f.Close()

	if err := writer.Close(); err != nil {
		return err
	}

	return nil
}

func createV2(
	path string,
	files []string,
	dirs []string,
	outputDir string,
	numWorkers int,
	chunkSize uint32) (err error) {

	writer := archive.ArchiveWriterV2{}
	if err := writer.CreateArchive(path, outputDir, chunkSize); err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

	// f, err := os.Create("compress.prof")
	// if err != nil {
	// 	return err
	// }

	// if err := pprof.StartCPUProfile(f); err != nil {
	// 	f.Close()
	// 	return err
	// }

	defer func() {
		if err != nil {
			writer.OutputFile.Close()
			remErr := os.Remove(writer.OutputFile.Name())
			err = errors.Join(err, remErr)
		}
	}()

	fileEntries := make([]archive.FileStruct, len(files))

	for i, file := range files {
		fileEntries[i] = archive.FileStruct{Path: file, Method: archive.Default}
	}

	if err := writer.AddFilesConcurrent(fileEntries, numWorkers); err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

	for _, dir := range dirs {
		if err := writer.AddDirectory(dir, numWorkers); err != nil {
			return fmt.Errorf("CREATE::%w", err)
		}
	}

	// if err := writer.AddFilesConcurrent(fileEntries, numWorkers); err != nil {
	// 	pprof.StopCPUProfile()
	// 	f.Close()
	// 	return fmt.Errorf("CREATE::%w", err)
	// }

	// pprof.StopCPUProfile()
	// f.Close()

	if err := writer.Close(); err != nil {
		return err
	}

	return nil
}

func create(
	path string,
	files []string,
	dirs []string,
	outputDir string,
	version int,
	numWorkers int,
	chunkSize uint32) (err error) {

	if numWorkers == -1 {
		numWorkers = 8
	} else if numWorkers <= 0 {
		return fmt.Errorf("CREATE::number of workers must be positive")
	}

	if chunkSize <= 0 || chunkSize > archive.MAXCHUNKSIZE/KB {
		return fmt.Errorf("CREATE::invalid chunk size: %d", chunkSize)
	}

	switch version {
	case 1:
		return createV1(path, files, dirs, outputDir, numWorkers)
	case 2:
		return createV2(path, files, dirs, outputDir, numWorkers, chunkSize)
	default:
		return fmt.Errorf("CREATE::invalid writer version")
	}
}
