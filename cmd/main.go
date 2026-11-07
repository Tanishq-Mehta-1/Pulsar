package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		handle_invalid("")
		return
	}

	//profiling
	// f, err := os.Create("cpu.prof")
	// if err != nil {
	// 	panic(err)
	// }
	// defer f.Close()

	// if err := pprof.StartCPUProfile(f); err != nil {
	// 	panic(err)
	// }
	// defer pprof.StopCPUProfile()

	arguments := os.Args[1:]

	if arguments[0] == "help" {
		command := ""
		if len(arguments) > 1 {
			command = arguments[1]
		}
		handle_invalid(command)
		return
	}

	if len(arguments) < 2 {
		handle_invalid("")
		return
	}

	switch arguments[0] {

	case "list":
		archivePath := arguments[1]
		if err := list(archivePath); err != nil {
			log.Fatal(fmt.Errorf("PLSR::LIST::%w", err))
		}

	case "extract":
		archivePath := arguments[1]

		outputDir := ""
		numWorkers := -1

		if len(arguments) > 2 {

			for i, n := 2, len(arguments); i < n; i++ {
				arg := arguments[i]

				if arg == "-o" && i+1 < n {
					outputDir = arguments[i+1]
					i++
				} else if arg == "--workers" && i+1 < n {
					var err error
					numWorkers, err = strconv.Atoi(arguments[i+1])
					if err != nil {
						log.Fatal(fmt.Errorf("PLSR::EXTRACT::%w", err))
					}
					i++
				} else {
					handle_invalid(arguments[0])
					return
				}
			}
		}

		if err := extract(archivePath, outputDir, numWorkers); err != nil {
			log.Fatal(fmt.Errorf("PLSR::%w", err))
		}

	case "create":
		if len(arguments) <= 2 {
			handle_invalid(arguments[0])
			return
		}
		archivePath := arguments[1]

		var dirs, files []string
		numWorkers := -1
		outputDir := ""
		version := 1
		takingFiles := true

		for i, n := 2, len(arguments); i < n; i++ {
			arg := arguments[i]

			if arg == "--workers" && i+1 < n {
				takingFiles = false
				var err error
				numWorkers, err = strconv.Atoi(arguments[i+1])
				if err != nil {
					log.Fatal(fmt.Errorf("PLSR::CREATE::%w", err))
				}
				i++
			} else if arg == "-o" && i+1 < n {
				takingFiles = false
				outputDir = arguments[i+1]
				i++

			} else if arg == "-v" && i+1 < n {
				takingFiles = false
				var err error
				version, err = strconv.Atoi(arguments[i+1])
				if err != nil {
					log.Fatal(fmt.Errorf("PLSR::CREATE::%w", err))
				}
				i++
			} else {
				if !takingFiles {
					handle_invalid(arguments[0])
					return
				}

				var err error
				if files, dirs, err = processArg(arg, files, dirs); err != nil {
					log.Fatal(fmt.Errorf("PLSR::CREATE::%w", err))
				}
			}
		}

		if err := create(archivePath, files, dirs, outputDir, numWorkers, version); err != nil {
			log.Fatal(fmt.Errorf("PLSR::%w", err))
		}

	case "info":

		if len(arguments) != 2 {
			handle_invalid("info")
			return
		}

		info, err := getArchiveInfo(arguments[1])
		if err != nil {
			log.Fatal(err)
		}

		printArchiveInfo(info)

	default:
		handle_invalid(arguments[0])
	}

	// test_writer()
	// test_reader()
}
