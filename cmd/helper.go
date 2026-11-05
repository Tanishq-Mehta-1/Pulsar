package main

import "fmt"

var commandDocs = map[string]string{
	"create": `
Usage:
  plsr create <archive> <input...> [--workers N]

Description:
  Create a new archive from files and directories.

Examples:
  plsr create backup.fz ./project
  plsr create backup.fz file1.txt file2.txt
  plsr create backup.fz ./project --workers 8
`,

	"extract": `
Usage:
  plsr extract <archive> [-o <directory>] [--workers N]

Description:
  Extract an archive.

Options:
  -o          Output directory (default: current directory)
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
  Display the contents of an archive.

Example:
  plsr list backup.fz
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
