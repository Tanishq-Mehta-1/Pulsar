package main

import "fmt"

var commandDocs = map[string]string{
	"create": `
Usage:
  plsr create <archive> <input...> [-o <directory>] [--workers N]

Description:
  Create a new lossless archive from files and directories.
  Supports multiple input files and directories, preserving directory structure.

Options:
  -o          Directory in which to create the archive (default: current directory)
  --workers   Number of compression workers (default: 8)

Examples:
  plsr create backup.fz ./project
  plsr create backup.fz file1.txt file2.txt
  plsr create backup.fz ./project ./notes.txt
  plsr create backup.fz ./project --workers 8
  plsr create backup.fz ./project -o ./archives
`,

	"extract": `
Usage:
  plsr extract <archive> [-o <directory>] [--workers N]

Description:
  Extract files and directories from an archive.
  Preserves nested directory structures and empty directories.

Options:
  -o          Extraction destination (default: current directory)
  --workers   Number of decompression workers (default: 8)

Examples:
  plsr extract backup.fz
  plsr extract backup.fz -o ./output
  plsr extract backup.fz --workers 8
`,

	"list": `
Usage:
  plsr list <archive>

Description:
  Display the contents of an archive in a hierarchical tree structure.
  Shows files, nested directories, and empty directories.

Example:
  plsr list backup.fz
`, "info": `
Usage:
  plsr info <archive>

Description:
  Display metadata and statistics about an archive.
  Shows archive size, original size, compressed size,
  compression ratio, entry counts, and compression methods.

Example:
  plsr info backup.fz
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
