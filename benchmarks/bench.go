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

type Benchmark func(archivePath, input, output string) (time.Duration, time.Duration, int64, error)

func runCommandDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, out)
	}
	return nil
}

func requireTools(names ...string) map[string]bool {
	available := make(map[string]bool, len(names))
	for _, name := range names {
		_, err := exec.LookPath(name)
		available[name] = err == nil
		if err != nil {
			fmt.Printf("Skipping benchmarks requiring %q (not found in PATH)\n", name)
		}
	}
	return available
}
func runPulsar(binary, archivePath, input, output string, workers int) (time.Duration, time.Duration, int64, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	// Pulsar automatically appends .fz
	archiveName := strings.TrimSuffix(
		filepath.Base(archivePath),
		filepath.Ext(archivePath),
	)
	actualArchivePath := filepath.Join(
		filepath.Dir(archivePath),
		archiveName+".fz",
	)

	_ = os.Remove(actualArchivePath)
	defer os.Remove(actualArchivePath)

	start := time.Now()
	cmd := exec.Command(
		binary, "create", archiveName, input,
		"-o", filepath.Dir(archivePath),
		"--workers", fmt.Sprint(workers),
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, 0, fmt.Errorf("Pulsar create: %w\n%s", err, out)
	}
	compressTime := time.Since(start)

	info, err := os.Stat(actualArchivePath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("expected archive %s: %w", actualArchivePath, err)
	}

	start = time.Now()
	cmd = exec.Command(
		binary, "extract", actualArchivePath,
		"-o", output, "--workers", fmt.Sprint(workers),
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, 0, fmt.Errorf("Pulsar extract: %w\n%s", err, out)
	}

	return compressTime, time.Since(start), info.Size(), nil
}

func createGoZip(input, archivePath string) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)

	root := filepath.Dir(input)
	err = filepath.Walk(input, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		dst, err := writer.CreateHeader(header)
		if err != nil || info.IsDir() {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
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

func extractGoZip(archivePath, output string) error {
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
			_ = src.Close()
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

func benchmarkGoZip(archivePath, input, output string) (time.Duration, time.Duration, int64, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)
	start := time.Now()
	if err := createGoZip(input, archivePath); err != nil {
		return 0, 0, 0, err
	}
	compressTime := time.Since(start)
	info, err := os.Stat(archivePath)
	if err != nil {
		return 0, 0, 0, err
	}
	start = time.Now()
	if err := extractGoZip(archivePath, output); err != nil {
		return 0, 0, 0, err
	}
	return compressTime, time.Since(start), info.Size(), nil
}

func benchmarkExternal(
	archivePath, input, output, workDir string,
	createArgs, extractArgs func(archivePath, input, output string) []string,
) (time.Duration, time.Duration, int64, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	start := time.Now()
	create := createArgs(archivePath, input, output)
	if err := runCommandDir(workDir, create[0], create[1:]...); err != nil {
		return 0, 0, 0, err
	}
	compressTime := time.Since(start)

	info, err := os.Stat(archivePath)
	if err != nil {
		return 0, 0, 0, err
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return 0, 0, 0, err
	}
	start = time.Now()
	args := extractArgs(archivePath, input, output)
	if err := runCommandDir(workDir, args[0], args[1:]...); err != nil {
		return 0, 0, 0, err
	}
	return compressTime, time.Since(start), info.Size(), nil
}

func medianDuration(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func medianInt64(values []int64) int64 {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
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
	fmt.Printf("%-15s %-8d %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f\n",
		r.Name, r.Workers,
		float64(r.ArchiveSize)/(1024*1024),
		r.Compress.Seconds(), r.Extract.Seconds(), ratio,
		compressSpeed, extractSpeed)
}

func runRepeated(name string, workers int, runs int, originalSize int64,
	tempDir, input string, benchmark Benchmark) (resultErr error) {
	var compressTimes, extractTimes []time.Duration
	var archiveSizes []int64

	for run := 0; run < runs; run++ {
		archivePath := filepath.Join(tempDir, fmt.Sprintf("bench-%s-%d.archive", strings.ReplaceAll(name, " ", "-"), run))
		output := filepath.Join(tempDir, fmt.Sprintf("out-%s-%d", strings.ReplaceAll(name, " ", "-"), run))

		// Always remove benchmark artifacts, including when compression or
		// extraction fails. The archive must remain until extraction finishes.
		func() {
			defer os.RemoveAll(output)
			defer os.Remove(archivePath)

			c, e, size, err := benchmark(archivePath, input, output)
			if err != nil {
				resultErr = fmt.Errorf("%s run %d: %w", name, run+1, err)
				return
			}
			compressTimes = append(compressTimes, c)
			extractTimes = append(extractTimes, e)
			archiveSizes = append(archiveSizes, size)
		}()
		if resultErr != nil {
			return resultErr
		}
	}

	printResult(Result{
		Name: name, Workers: workers,
		ArchiveSize: medianInt64(archiveSizes),
		Compress:    medianDuration(compressTimes),
		Extract:     medianDuration(extractTimes),
	}, originalSize)
	return nil
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
	if _, err := os.Stat(binaryPath); err != nil {
		panic(fmt.Errorf("Pulsar binary not found: %w", err))
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

	available := requireTools("zip", "unzip", "7z", "pigz", "zstd", "tar")
	fmt.Printf("Dataset: %s\nOriginal size: %.2f MB\nRuns: %d\n\n",
		inputPath, float64(originalSize)/(1024*1024), *runs)
	fmt.Printf("%-15s %-8s %-12s %-12s %-12s %-12s %-12s %-12s\n",
		"Format", "Workers", "Archive MB", "Compress s", "Extract s",
		"Ratio %", "Comp MB/s", "Extract MB/s")

	pulsarBench := func(workers int) Benchmark {
		return func(a, i, o string) (time.Duration, time.Duration, int64, error) {
			return runPulsar(binaryPath, a, i, o, workers)
		}
	}
	for _, workers := range []int{1, 2, 4, 8, 16, 64} {
		if err := runRepeated(fmt.Sprintf("Pulsar-%d", workers), workers, *runs,
			originalSize, tempDir, inputPath, pulsarBench(workers)); err != nil {
			panic(err)
		}
	}

	if err := runRepeated("Go ZIP", 1, *runs, originalSize, tempDir, inputPath,
		benchmarkGoZip); err != nil {
		panic(err)
	}

	parent, base := filepath.Dir(inputPath), filepath.Base(inputPath)

	if available["zip"] && available["unzip"] {
		bench := func(a, i, o string) (time.Duration, time.Duration, int64, error) {
			return benchmarkExternal(a, i, o, parent,
				func(a, _, _ string) []string { return []string{"zip", "-q", "-r", "-6", a, base} },
				func(a, _, o string) []string { return []string{"unzip", "-q", a, "-d", o} })
		}
		if err := runRepeated("System ZIP", 1, *runs, originalSize, tempDir, inputPath, bench); err != nil {
			panic(err)
		}
	}

	if available["7z"] {
		for _, mode := range []struct {
			name string
			args []string
		}{
			{"7z Deflate", []string{"-tzip", "-mm=Deflate", "-mx=6", "-mmt=on"}},
			{"7z LZMA2", []string{"-t7z", "-m0=lzma2", "-mx=5", "-mmt=on"}},
		} {
			mode := mode
			bench := func(a, i, o string) (time.Duration, time.Duration, int64, error) {
				return benchmarkExternal(a, i, o, parent,
					func(a, _, _ string) []string {
						args := []string{"7z", "a", "-y"}
						args = append(args, mode.args...)
						return append(args, a, base)
					},
					func(a, _, o string) []string { return []string{"7z", "x", "-y", "-o" + o, a} })
			}
			if err := runRepeated(mode.name, 1, *runs, originalSize, tempDir, inputPath, bench); err != nil {
				panic(err)
			}
		}
	}

	if available["pigz"] && available["tar"] {
		bench := func(a, _, o string) (time.Duration, time.Duration, int64, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-I", "pigz -6", "-cf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-I", "pigz -d", "-xf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + pigz", 0, *runs, originalSize, tempDir, inputPath, bench); err != nil {
			panic(err)
		}
	}

	if available["zstd"] && available["tar"] {
		bench := func(a, _, o string) (time.Duration, time.Duration, int64, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-I", "zstd -T0 -3", "-cf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-I", "zstd -d", "-xf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + zstd", 0, *runs, originalSize, tempDir, inputPath, bench); err != nil {
			panic(err)
		}
	}
}
