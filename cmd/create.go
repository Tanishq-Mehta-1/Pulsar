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

func v1MethodFromCodec(codecStr string) (archive.CompressionMethod, error) {
	codec, err := archive.ParseCodec(codecStr)
	if err != nil {
		return archive.Default, err
	}
	switch codec {
	case archive.CodecStored:
		return archive.None, nil
	case archive.CodecDeflate:
		return archive.Default, nil
	case archive.CodecZstd:
		return archive.Zstd, nil
	default:
		return archive.Default, fmt.Errorf("unsupported codec: %s", codecStr)
	}
}

func createV1(
	path string,
	files []string,
	dirs []string,
	outputDir string,
	numWorkers int,
	codecStr string) (err error) {
	method, err := v1MethodFromCodec(codecStr)
	if err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

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
		fileEntries[i] = archive.FileStruct{Path: file, Method: method}
	}

	if err := writer.AddFilesConcurrent(fileEntries, numWorkers); err != nil {
		return fmt.Errorf("CREATE::%w", err)
	}

	for _, dir := range dirs {
		if err := writer.AddDirectory(dir, numWorkers, method); err != nil {
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
	chunkSize uint32,
	codec_str string,
	preset archive.CompressionMethod) (err error) {

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
		fileEntries[i] = archive.FileStruct{Path: file, Method: preset}
	}

	codec, err := archive.ParseCodec(codec_str)
	if err != nil {
		return fmt.Errorf("CREATE::%v", err)
	}

	if err := writer.AddFilesConcurrent(fileEntries, numWorkers, codec); err != nil {
		return fmt.Errorf("CREATE::%v", err)
	}

	for _, dir := range dirs {
		if err := writer.AddDirectory(dir, numWorkers, codec, preset); err != nil {
			return fmt.Errorf("CREATE::%v", err)
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
	chunkSize uint32,
	codec string,
	preset archive.CompressionMethod) (err error) {

	if numWorkers < 0 {
		return fmt.Errorf("CREATE::invalid worker count")
	} else if numWorkers == 0 {
		numWorkers = archive.ResolveWorkers(numWorkers)
		// fmt.Printf("Workers assigned: %d", numWorkers)
	}

	if chunkSize > archive.MAXCHUNKSIZE/KB {
		return fmt.Errorf("CREATE::invalid chunk-size: %d, must be in range [0, %d]", chunkSize, archive.MAXCHUNKSIZE/KB)
	}

	switch version {
	case 1:
		return createV1(path, files, dirs, outputDir, numWorkers, codec)
	case 2:
		return createV2(path, files, dirs, outputDir, numWorkers, chunkSize*KB, codec)
	default:
		return fmt.Errorf("CREATE::invalid writer version")
	}
}
