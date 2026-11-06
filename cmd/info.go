package main

import (
	"fmt"
	"os"
	"strings"

	"pulsar/internal/archive"
)

type ArchiveInfo struct {
	Path               string
	ArchiveSize        int64
	OriginalSize       uint64
	CompressedSize     uint64
	FileCount          int
	DirectoryCount     int
	CompressionMethods []string
}

func getArchiveInfo(path string) (info ArchiveInfo, err error) {
	reader := archive.ArchiveReader{}

	if err := reader.OpenArchive(path); err != nil {
		return info, fmt.Errorf("INFO::%w", err)
	}
	defer reader.Close()

	stat, err := os.Stat(path)
	if err != nil {
		return info, fmt.Errorf("INFO::%w", err)
	}

	info.Path = path
	info.ArchiveSize = stat.Size()

	methods := make(map[archive.CompressionMethod]bool)

	for _, entry := range reader.Files {
		switch entry.Type {
		case archive.File:
			info.FileCount++
			info.OriginalSize += entry.OriginalSize
			info.CompressedSize += entry.CompressedSize
			methods[entry.CompressionMethod] = true

		case archive.Directory:
			info.DirectoryCount++
		}
	}

	for method := range methods {
		info.CompressionMethods = append(
			info.CompressionMethods,
			fmt.Sprint(method),
		)
	}

	return info, nil
}

func printArchiveInfo(info ArchiveInfo) {
	fmt.Println("Pulsar Archive Information")
	fmt.Println(strings.Repeat("─", 38))

	fmt.Printf("%-20s %s\n", "Archive:", info.Path)
	fmt.Printf("%-20s %s\n", "Archive Size:", formatSize(uint64(info.ArchiveSize)))
	fmt.Printf("%-20s %s\n", "Original Size:", formatSize(info.OriginalSize))
	fmt.Printf("%-20s %s\n", "Compressed Size:", formatSize(info.CompressedSize))

	if info.OriginalSize > 0 {
		ratio := float64(info.CompressedSize) / float64(info.OriginalSize) * 100
		fmt.Printf("%-20s %.2f%%\n", "Compression Ratio:", ratio)
	}

	fmt.Println("\nContents")
	fmt.Println(strings.Repeat("─", 38))

	fmt.Printf("%-20s %d\n", "Files:", info.FileCount)
	fmt.Printf("%-20s %d\n", "Directories:", info.DirectoryCount)
	fmt.Printf("%-20s %d\n", "Total Entries:", info.FileCount+info.DirectoryCount)

	fmt.Println("\nCompression")
	fmt.Println(strings.Repeat("─", 38))

	fmt.Printf("%-20s %s\n", "Methods:", strings.Join(info.CompressionMethods, ", "))
	fmt.Printf("%-20s %s\n", "Integrity:", "CRC32 (IEEE)")
}

func formatSize(size uint64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := uint64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %ciB",
		float64(size)/float64(div),
		"KMGTPE"[exp],
	)
}
