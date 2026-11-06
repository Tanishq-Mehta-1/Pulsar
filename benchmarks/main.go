package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Result struct {
	Name        string
	Workers     int
	ArchiveSize int64
	Compress    time.Duration
	Extract     time.Duration
}

func runPulsar(binary, archivePath, input, output string, workers int) (time.Duration, time.Duration, int64, error) {
	os.Remove(archivePath)
	os.RemoveAll(output)

	start := time.Now()

	archiveName := strings.TrimSuffix(
		filepath.Base(archivePath),
		filepath.Ext(archivePath),
	)

	cmd := exec.Command(
		binary,
		"create",
		archiveName,
		input,
		"-o", filepath.Dir(archivePath),
		"--workers", fmt.Sprint(workers),
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, 0, fmt.Errorf("Pulsar create: %w\n%s", err, out)
	}
	compressTime := time.Since(start)

	info, err := os.Stat(archivePath)
	if err != nil {
		return 0, 0, 0, err
	}
	archiveSize := info.Size()

	start = time.Now()
	cmd = exec.Command(binary, "extract", archivePath,
		"-o", output, "--workers", fmt.Sprint(workers))
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, 0, fmt.Errorf("Pulsar extract: %w\n%s", err, out)
	}
	extractTime := time.Since(start)

	return compressTime, extractTime, archiveSize, nil
}

func createZip(input, archivePath string) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}

	writer := zip.NewWriter(file)

	root := filepath.Dir(input)

	err = filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		name := filepath.ToSlash(rel)

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = name
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		if info.IsDir() {
			_, err = writer.CreateHeader(header)
			return err
		}

		dst, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		_, err = io.Copy(dst, src)
		return err
	})

	closeErr := writer.Close()
	fileErr := file.Close()

	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return fileErr
}

func extractZip(archivePath, output string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if !filepath.IsLocal(filepath.FromSlash(file.Name)) {
			return fmt.Errorf("invalid ZIP path: %s", file.Name)
		}

		dst := filepath.Join(output, filepath.FromSlash(file.Name))

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		src, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			src.Close()
			return err
		}

		_, copyErr := io.Copy(out, src)
		srcErr := src.Close()
		outErr := out.Close()

		if copyErr != nil {
			return copyErr
		}
		if srcErr != nil {
			return srcErr
		}
		if outErr != nil {
			return outErr
		}
	}

	return nil
}

func benchmarkZip(archivePath, input, output string) (time.Duration, time.Duration, int64, error) {
	os.Remove(archivePath)
	os.RemoveAll(output)

	start := time.Now()
	if err := createZip(input, archivePath); err != nil {
		return 0, 0, 0, err
	}
	compressTime := time.Since(start)

	info, err := os.Stat(archivePath)
	if err != nil {
		return 0, 0, 0, err
	}

	start = time.Now()
	if err := extractZip(archivePath, output); err != nil {
		return 0, 0, 0, err
	}
	extractTime := time.Since(start)

	return compressTime, extractTime, info.Size(), nil
}

func median(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	return sorted[len(sorted)/2]
}

func totalSize(root string) (int64, error) {
	var total int64

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})

	return total, err
}

func printResult(r Result, originalSize int64) {
	ratio := float64(r.ArchiveSize) / float64(originalSize) * 100
	compressSpeed := float64(originalSize) / r.Compress.Seconds() / (1024 * 1024)
	extractSpeed := float64(originalSize) / r.Extract.Seconds() / (1024 * 1024)

	fmt.Printf("%-10s %-8d %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f\n",
		r.Name, r.Workers,
		float64(r.ArchiveSize)/(1024*1024),
		r.Compress.Seconds(),
		r.Extract.Seconds(),
		ratio,
		compressSpeed,
		extractSpeed,
	)
	_ = extractSpeed
}

func main() {
	binary := flag.String("plsr", "./plsr", "Path to Pulsar executable")
	input := flag.String("input", "", "Input directory to benchmark")
	runs := flag.Int("runs", 3, "Number of benchmark repetitions")
	flag.Parse()

	if *input == "" || *runs <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath, err := filepath.Abs(*input)
	if err != nil {
		panic(err)
	}

	info, err := os.Stat(inputPath)
	if err != nil || !info.IsDir() {
		fmt.Println("Input must be an existing directory")
		os.Exit(1)
	}

	binaryPath, err := filepath.Abs(*binary)
	if err != nil {
		panic(err)
	}

	originalSize, err := totalSize(inputPath)
	if err != nil {
		panic(err)
	}
	if originalSize == 0 {
		fmt.Println("Dataset contains no data")
		os.Exit(1)
	}

	tempDir, err := os.MkdirTemp("", "pulsar-benchmark-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	fmt.Printf("Dataset: %s\n", inputPath)
	fmt.Printf("Original size: %.2f MB\n", float64(originalSize)/(1024*1024))
	fmt.Printf("Runs: %d\n\n", *runs)

	fmt.Printf("%-10s %-8s %-12s %-12s %-12s %-12s %-12s\n",
		"Format", "Workers", "Archive MB", "Compress s", "Extract s", "Ratio %", "Comp MB/s")

	workersList := []int{1, 2, 4, 8, 16}

	for _, workers := range workersList {
		var compressTimes, extractTimes []time.Duration
		var archiveSize int64

		for run := 0; run < *runs; run++ {
			archivePath := filepath.Join(tempDir, "pulsar.fz")
			output := filepath.Join(tempDir, "pulsar-output")

			c, e, size, err := runPulsar(binaryPath, archivePath, inputPath, output, workers)
			if err != nil {
				panic(err)
			}

			compressTimes = append(compressTimes, c)
			extractTimes = append(extractTimes, e)
			archiveSize = size
		}

		result := Result{
			Name:        "Pulsar",
			Workers:     workers,
			ArchiveSize: archiveSize,
			Compress:    median(compressTimes),
			Extract:     median(extractTimes),
		}
		printResult(result, originalSize)
	}

	var compressTimes, extractTimes []time.Duration
	var archiveSize int64

	for run := 0; run < *runs; run++ {
		archivePath := filepath.Join(tempDir, "benchmark.zip")
		output := filepath.Join(tempDir, "zip-output")

		c, e, size, err := benchmarkZip(archivePath, inputPath, output)
		if err != nil {
			panic(err)
		}

		compressTimes = append(compressTimes, c)
		extractTimes = append(extractTimes, e)
		archiveSize = size
	}

	result := Result{
		Name:        "ZIP",
		Workers:     1,
		ArchiveSize: archiveSize,
		Compress:    median(compressTimes),
		Extract:     median(extractTimes),
	}
	printResult(result, originalSize)
}
