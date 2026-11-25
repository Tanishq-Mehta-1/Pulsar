package main

import "fmt"

var commandDocs = map[string]string{
	"create": `
Usage:
  plsr create <archive> <input...> [-o <directory>] [--workers N] [-v <1|2>] [--chunk-size <KB>] [--codec <zstd|deflate|stored>]

Description:
  Create a new lossless archive from files and directories.
  Supports multiple input files and directories, preserving directory structure.

Options:
  -o            Directory in which to create the archive (default: current directory)
  -v            Archive format version: 1 (file-level) or 2 (chunk-based) (default: 2)
  --workers     Number of compression workers (default: 0, assigns number of cores)
  --chunk-size  Chunk size in KB for v2 archives (default: 4096, 0 for dynamic file chunking)
  --codec       Compression codec for v2 archives: zstd, deflate, or stored (default: zstd)

Examples:
  plsr create backup.plsr ./project
  plsr create backup.plsr ./project -v 2 --codec zstd --chunk-size 4096 --workers 12
  plsr create backup.plsr ./project -v 2 --codec deflate --chunk-size 256 --workers 8
  plsr create backup.plsr file1.txt file2.txt -o ./archives
`,

	"extract": `
Usage:
  plsr extract <archive> [-o <directory>] [--workers N]

Description:
  Extract files and directories from an archive (auto-detects v1/v2 and codec).
  Preserves nested directory structures and empty directories.

Options:
  -o          Extraction destination (default: current directory)
  --workers   Number of decompression workers (default: 0, assigns number of cores)

Examples:
  plsr extract backup.plsr
  plsr extract backup.plsr -o ./output
  plsr extract backup.plsr --workers 8
`,

	"list": `
Usage:
  plsr list <archive>

Description:
  Display the contents of an archive in a hierarchical tree structure.
  Shows files, nested directories, and empty directories.

Example:
  plsr list backup.plsr
`,

	"info": `
Usage:
  plsr info <archive>

Description:
  Display metadata and statistics about an archive.
  Shows format version, sizes, compression ratio, entry/chunk counts, and methods.

Example:
  plsr info backup.plsr
`,
}

func handle_invalid(command string) {
	if command == "" || command == "help" {
		fmt.Println("Pulsar - Lossless Archive Utility")
		fmt.Println("\nUsage: plsr <command> [arguments]")
		fmt.Println("\nAvailable commands:")

		for name := range commandDocs {
			fmt.Printf("  %-10s\n", name)
		}

		fmt.Println("\nUse 'plsr help <command>' for more information.")
		return
	}

	doc, exists := commandDocs[command]
	if !exists {
		fmt.Printf("Unknown command: %s\n", command)
		handle_invalid("")
		return
	}

	fmt.Println(doc)
}
