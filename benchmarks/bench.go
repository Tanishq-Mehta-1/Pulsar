package main

import (
	"archive/zip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Result struct {
	Name        string
	Workers     int
	ChunkKB     int
	ShowDyn     bool
	ArchiveSize int64
	Compress    time.Duration
	Extract     time.Duration
}

type RunMetrics struct {
	Compress    time.Duration
	Extract     time.Duration
	ArchiveSize int64
	PeakRSSMB   float64
}

type Benchmark func(archivePath, input, output string) (RunMetrics, error)

func maxRSSMB(cmd *exec.Cmd) float64 {
	if cmd == nil || cmd.ProcessState == nil {
		return 0
	}
	if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && ru != nil {
		// On Linux, Maxrss is reported in kilobytes.
		return float64(ru.Maxrss) / 1024.0
	}
	return 0
}

func runCommandDir(dir, name string, args ...string) (float64, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("%s %v: %w\n%s", name, args, err, out)
	}
	return maxRSSMB(cmd), nil
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

func fileSHA256(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func collectManifest(root string) (map[string][32]byte, error) {
	manifest := make(map[string][32]byte)
	base := filepath.Base(filepath.Clean(root))
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(base, rel))
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		manifest[key] = sum
		return nil
	})
	return manifest, err
}

func verifyExtracted(manifest map[string][32]byte, outputDir string) error {
	for rel, expected := range manifest {
		extractedPath := filepath.Join(outputDir, filepath.FromSlash(rel))
		actual, err := fileSHA256(extractedPath)
		if err != nil {
			return fmt.Errorf("verify %s: %w", rel, err)
		}
		if actual != expected {
			return fmt.Errorf("verify %s: SHA-256 mismatch", rel)
		}
	}
	return nil
}

func runPulsar(binary, archivePath, input, output string, version int, codec string, chunkSizeKB, workers int) (RunMetrics, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	archiveName := strings.TrimSuffix(
		filepath.Base(archivePath),
		filepath.Ext(archivePath),
	)
	actualArchivePath := filepath.Join(
		filepath.Dir(archivePath),
		archiveName+".plsr",
	)

	_ = os.Remove(actualArchivePath)
	defer os.Remove(actualArchivePath)

	createArgs := []string{
		"create", archiveName, input,
		"-o", filepath.Dir(archivePath),
		"--workers", fmt.Sprint(workers),
		"-v", fmt.Sprint(version),
		"--codec", codec,
	}
	if version == 2 {
		createArgs = append(createArgs, "--chunk-size", fmt.Sprint(chunkSizeKB))
	}

	start := time.Now()
	createCmd := exec.Command(binary, createArgs...)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return RunMetrics{}, fmt.Errorf("Pulsar v%d (%s) create: %w\n%s", version, codec, err, out)
	}
	compressTime := time.Since(start)
	createRSS := maxRSSMB(createCmd)

	info, err := os.Stat(actualArchivePath)
	if err != nil {
		return RunMetrics{}, fmt.Errorf("expected archive %s: %w", actualArchivePath, err)
	}

	start = time.Now()
	extractCmd := exec.Command(
		binary, "extract", actualArchivePath,
		"-o", output, "--workers", fmt.Sprint(workers),
	)
	if out, err := extractCmd.CombinedOutput(); err != nil {
		return RunMetrics{}, fmt.Errorf("Pulsar v%d (%s) extract: %w\n%s", version, codec, err, out)
	}
	extractTime := time.Since(start)
	extractRSS := maxRSSMB(extractCmd)

	peakRSS := createRSS
	if extractRSS > peakRSS {
		peakRSS = extractRSS
	}

	return RunMetrics{
		Compress:    compressTime,
		Extract:     extractTime,
		ArchiveSize: info.Size(),
		PeakRSSMB:   peakRSS,
	}, nil
}

func benchmarkExternal(
	archivePath, input, output, workDir string,
	createArgs, extractArgs func(archivePath, input, output string) []string,
) (RunMetrics, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	start := time.Now()
	create := createArgs(archivePath, input, output)
	createRSS, err := runCommandDir(workDir, create[0], create[1:]...)
	if err != nil {
		return RunMetrics{}, err
	}
	compressTime := time.Since(start)

	info, err := os.Stat(archivePath)
	if err != nil {
		return RunMetrics{}, err
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return RunMetrics{}, err
	}
	start = time.Now()
	args := extractArgs(archivePath, input, output)
	extractRSS, err := runCommandDir(workDir, args[0], args[1:]...)
	if err != nil {
		return RunMetrics{}, err
	}
	extractTime := time.Since(start)

	peakRSS := createRSS
	if extractRSS > peakRSS {
		peakRSS = extractRSS
	}

	return RunMetrics{
		Compress:    compressTime,
		Extract:     extractTime,
		ArchiveSize: info.Size(),
		PeakRSSMB:   peakRSS,
	}, nil
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

func benchmarkGoZip(archivePath, input, output string) (RunMetrics, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)
	start := time.Now()
	if err := createGoZip(input, archivePath); err != nil {
		return RunMetrics{}, err
	}
	compressTime := time.Since(start)
	info, err := os.Stat(archivePath)
	if err != nil {
		return RunMetrics{}, err
	}
	start = time.Now()
	if err := extractGoZip(archivePath, output); err != nil {
		return RunMetrics{}, err
	}
	return RunMetrics{
		Compress:    compressTime,
		Extract:     time.Since(start),
		ArchiveSize: info.Size(),
	}, nil
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

func maxFloat64(values []float64) float64 {
	var m float64
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
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

func parseIntList(raw string, allowZero bool) ([]int, error) {
	parts := strings.Split(raw, ",")
	vals := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		val, err := strconv.Atoi(p)
		if err != nil || val < 0 || (!allowZero && val == 0) {
			return nil, fmt.Errorf("invalid integer value %q", p)
		}
		vals = append(vals, val)
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("at least one value is required")
	}
	return vals, nil
}

func parseStringList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printResult(r Result, originalSize int64) {
	ratio := float64(r.ArchiveSize) / float64(originalSize) * 100
	compressSpeed := float64(originalSize) / r.Compress.Seconds() / (1024 * 1024)
	extractSpeed := float64(originalSize) / r.Extract.Seconds() / (1024 * 1024)
	chunkCol := "-"
	if r.ShowDyn {
		chunkCol = "dyn"
	} else if r.ChunkKB > 0 {
		chunkCol = fmt.Sprintf("%d", r.ChunkKB)
	}
	workersCol := "Auto"
	if r.Workers > 0 {
		workersCol = fmt.Sprintf("%d", r.Workers)
	}
	fmt.Printf("%-18s %-8s %-9s %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f %-12.2f\n",
		r.Name, workersCol, chunkCol,
		float64(r.ArchiveSize)/(1024*1024),
		r.Compress.Seconds(), r.Extract.Seconds(), ratio,
		compressSpeed, extractSpeed)
}

func runRepeated(name string, workers int, chunkKB int, showDyn bool, runs int, originalSize int64, manifest map[string][32]byte,
	tempDir, input string, benchmark Benchmark) error {
	var compressTimes, extractTimes []time.Duration
	var archiveSizes []int64

	safeID := strings.NewReplacer(" ", "-", "/", "-", "(", "", ")", "", ",", "-").Replace(name)
	if showDyn {
		safeID = fmt.Sprintf("%s-dyn-%dw", safeID, workers)
	} else if chunkKB > 0 {
		safeID = fmt.Sprintf("%s-%dKB-%dw", safeID, chunkKB, workers)
	} else if workers > 0 {
		safeID = fmt.Sprintf("%s-%dw", safeID, workers)
	}

	for run := 0; run < runs; run++ {
		archivePath := filepath.Join(tempDir, fmt.Sprintf("bench-%s-%d.archive", safeID, run))
		output := filepath.Join(tempDir, fmt.Sprintf("out-%s-%d", safeID, run))

		var runErr error
		func() {
			defer os.RemoveAll(output)
			defer os.Remove(archivePath)

			m, err := benchmark(archivePath, input, output)
			if err != nil {
				runErr = fmt.Errorf("%s run %d: %w", name, run+1, err)
				return
			}
			if manifest != nil {
				if err := verifyExtracted(manifest, output); err != nil {
					runErr = fmt.Errorf("%s run %d verification failed: %w", name, run+1, err)
					return
				}
			}
			compressTimes = append(compressTimes, m.Compress)
			extractTimes = append(extractTimes, m.Extract)
			archiveSizes = append(archiveSizes, m.ArchiveSize)
		}()
		if runErr != nil {
			return runErr
		}
	}

	printResult(Result{
		Name:        name,
		Workers:     workers,
		ChunkKB:     chunkKB,
		ShowDyn:     showDyn,
		ArchiveSize: medianInt64(archiveSizes),
		Compress:    medianDuration(compressTimes),
		Extract:     medianDuration(extractTimes),
	}, originalSize)
	return nil
}

func parseSelectionSet(raw string, valid []string) (map[string]bool, error) {
	selected := make(map[string]bool, len(valid))
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" || trimmed == "none" {
		return selected, nil
	}
	if trimmed == "all" {
		for _, v := range valid {
			selected[v] = true
		}
		return selected, nil
	}

	validSet := make(map[string]bool, len(valid))
	for _, v := range valid {
		validSet[v] = true
	}
	for _, item := range parseStringList(trimmed) {
		if !validSet[item] {
			return nil, fmt.Errorf("unknown selection %q (valid: %v)", item, valid)
		}
		selected[item] = true
	}
	return selected, nil
}

func benchmarkDataset(binaryPath, inputRaw string, runs int, versions, codecs map[string]bool, chunkSizes, workerCounts []int, baselines map[string]bool, verify bool, available map[string]bool) error {
	inputPath, err := filepath.Abs(inputRaw)
	if err != nil {
		return err
	}
	info, err := os.Stat(inputPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("input %q must be an existing directory", inputRaw)
	}

	originalSize, err := totalSize(inputPath)
	if err != nil {
		return err
	}
	if originalSize == 0 {
		return fmt.Errorf("dataset %q contains no data", inputPath)
	}

	var manifest map[string][32]byte
	if verify {
		manifest, err = collectManifest(inputPath)
		if err != nil {
			return fmt.Errorf("failed to build verification manifest: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "pulsar-benchmark-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fmt.Printf("\nDataset: %s\nOriginal size: %.2f MB\nRuns: %d\nV2 Chunk sizes (KB): %v\n\n",
		inputPath, float64(originalSize)/(1024*1024), runs, chunkSizes)
	fmt.Printf("%-18s %-8s %-9s %-12s %-12s %-12s %-12s %-12s %-12s\n",
		"Format", "Workers", "Chunk KB", "Archive MB", "Compress s", "Extract s",
		"Ratio %", "Comp MB/s", "Extract MB/s")

	pulsarBench := func(version int, codec string, cKB, workers int) Benchmark {
		return func(a, i, o string) (RunMetrics, error) {
			return runPulsar(binaryPath, a, i, o, version, codec, cKB, workers)
		}
	}

	codecList := make([]string, 0, 3)
	for _, c := range []string{"deflate", "zstd", "stored"} {
		if codecs[c] {
			codecList = append(codecList, c)
		}
	}

	// Pulsar V1 matrix
	if versions["1"] {
		for _, codec := range codecList {
			for _, workers := range workerCounts {
				name := fmt.Sprintf("PulsarV1-%s", codec)
				if err := runRepeated(name, workers, 0, false, runs, originalSize, manifest, tempDir, inputPath, pulsarBench(1, codec, 0, workers)); err != nil {
					return err
				}
			}
		}
	}

	// Pulsar V2 matrix
	if versions["2"] {
		for _, cKB := range chunkSizes {
			showDyn := cKB == 0
			for _, codec := range codecList {
				for _, workers := range workerCounts {
					name := fmt.Sprintf("PulsarV2-%s", codec)
					if err := runRepeated(name, workers, cKB, showDyn, runs, originalSize, manifest, tempDir, inputPath, pulsarBench(2, codec, cKB, workers)); err != nil {
						return err
					}
				}
			}
		}
	}

	parent, base := filepath.Dir(inputPath), filepath.Base(inputPath)

	// Reference baselines
	if baselines["gozip"] {
		if err := runRepeated("Go ZIP", 1, 0, false, runs, originalSize, manifest, tempDir, inputPath, benchmarkGoZip); err != nil {
			return err
		}
	}

	if baselines["zip"] && available["zip"] && available["unzip"] {
		bench := func(a, i, o string) (RunMetrics, error) {
			return benchmarkExternal(a, i, o, parent,
				func(a, _, _ string) []string { return []string{"zip", "-q", "-r", "-6", a, base} },
				func(a, _, o string) []string { return []string{"unzip", "-q", a, "-d", o} })
		}
		if err := runRepeated("System ZIP", 1, 0, false, runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
			return err
		}
	}

	if (baselines["7z-deflate"] || baselines["7z-lzma2"]) && available["7z"] {
		for _, mode := range []struct {
			key  string
			name string
			args []string
		}{
			{"7z-deflate", "7z Deflate", []string{"-tzip", "-mm=Deflate", "-mx=6", "-mmt=on"}},
			{"7z-lzma2", "7z LZMA2", []string{"-t7z", "-m0=lzma2", "-mx=5", "-mmt=on"}},
		} {
			if !baselines[mode.key] {
				continue
			}
			mode := mode
			bench := func(a, i, o string) (RunMetrics, error) {
				return benchmarkExternal(a, i, o, parent,
					func(a, _, _ string) []string {
						args := []string{"7z", "a", "-y"}
						args = append(args, mode.args...)
						return append(args, a, base)
					},
					func(a, _, o string) []string { return []string{"7z", "x", "-y", "-o" + o, a} })
			}
			if err := runRepeated(mode.name, 1, 0, false, runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
				return err
			}
		}
	}

	if baselines["pigz"] {
		if available["tar"] && available["pigz"] {
			bench := func(a, _, o string) (RunMetrics, error) {
				return benchmarkExternal(a, inputPath, o, parent,
					func(a, _, _ string) []string {
						return []string{"tar", "-I", "pigz -6", "-cf", a, "-C", parent, base}
					},
					func(a, _, o string) []string {
						return []string{"tar", "-I", "pigz -d", "-xf", a, "-C", o}
					})
			}
			if err := runRepeated("tar + pigz", 0, 0, false, runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
				return err
			}
		} else if available["tar"] && available["gzip"] {
			bench := func(a, _, o string) (RunMetrics, error) {
				return benchmarkExternal(a, inputPath, o, parent,
					func(a, _, _ string) []string {
						return []string{"tar", "-czf", a, "-C", parent, base}
					},
					func(a, _, o string) []string {
						return []string{"tar", "-xzf", a, "-C", o}
					})
			}
			if err := runRepeated("tar + gzip", 0, 0, false, runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
				return err
			}
		}
	}

	if baselines["zstd"] && available["tar"] && available["zstd"] {
		bench := func(a, _, o string) (RunMetrics, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-I", "zstd -T0 -3", "-cf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-I", "zstd -d", "-xf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + zstd", 0, 0, false, runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	binary := flag.String("plsr", "./plsr", "Path to Pulsar executable")
	input := flag.String("input", "", "Comma-separated input directories to benchmark")
	runs := flag.Int("runs", 3, "Number of benchmark repetitions")
	versionsFlag := flag.String("versions", "1,2", "Comma-separated Pulsar versions to benchmark (1, 2, all, or none)")
	codecsFlag := flag.String("codecs", "deflate,zstd", "Comma-separated Pulsar codecs to benchmark (deflate, zstd, stored, all, or none)")
	chunkSizesFlag := flag.String("chunk-sizes", "1024,4096,8192,0", "Comma-separated chunk sizes in KB for Pulsar V2 (0 = dynamic)")
	workersFlag := flag.String("workers", "4,8,12", "Comma-separated worker counts to benchmark")
	baselinesFlag := flag.String("baselines", "all", "Comma-separated baselines to include (all, none, or subset of: gozip,zip,7z-deflate,7z-lzma2,pigz,zstd)")
	verify := flag.Bool("verify", true, "Verify extracted files SHA-256 byte-for-byte against input")
	flag.Parse()

	if *input == "" || *runs <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	datasets := parseStringList(*input)
	if len(datasets) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	versions, err := parseSelectionSet(*versionsFlag, []string{"1", "2"})
	if err != nil {
		panic(err)
	}
	codecs, err := parseSelectionSet(*codecsFlag, []string{"deflate", "zstd", "stored"})
	if err != nil {
		panic(err)
	}
	baselines, err := parseSelectionSet(*baselinesFlag, []string{"gozip", "zip", "7z-deflate", "7z-lzma2", "pigz", "zstd"})
	if err != nil {
		panic(err)
	}

	chunkSizes, err := parseIntList(*chunkSizesFlag, true)
	if err != nil {
		panic(err)
	}
	workerCounts, err := parseIntList(*workersFlag, false)
	if err != nil {
		panic(err)
	}

	binaryPath, err := filepath.Abs(*binary)
	if err != nil {
		panic(err)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		panic(fmt.Errorf("Pulsar binary not found: %w", err))
	}

	var neededTools []string
	if baselines["zip"] {
		neededTools = append(neededTools, "zip", "unzip")
	}
	if baselines["7z-deflate"] || baselines["7z-lzma2"] {
		neededTools = append(neededTools, "7z")
	}
	if baselines["pigz"] || baselines["zstd"] {
		neededTools = append(neededTools, "tar")
	}
	if baselines["pigz"] {
		neededTools = append(neededTools, "pigz", "gzip")
	}
	if baselines["zstd"] {
		neededTools = append(neededTools, "zstd")
	}
	available := requireTools(neededTools...)

	for _, ds := range datasets {
		if err := benchmarkDataset(binaryPath, ds, *runs, versions, codecs, chunkSizes, workerCounts, baselines, *verify, available); err != nil {
			panic(err)
		}
	}
}
