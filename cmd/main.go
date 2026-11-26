package main

import (
	"fmt"
	"log"
	"os"
	archive "pulsar/internal"
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
			log.Fatal(fmt.Errorf("PLSR::%w", err))
		}

	case "extract":
		archivePath := arguments[1]

		outputDir := ""
		numWorkers := 0

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

					if numWorkers < 0 {
						log.Fatal(fmt.Errorf("PLSR::EXTRACT::invalid worker count: %d", numWorkers))
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
		numWorkers := 0
		outputDir := ""
		version := 2
		chunkSize := 4096 //default 4MB
		codec := "zstd"
		preset := archive.Default
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

				if numWorkers < 0 {
					log.Fatal(fmt.Errorf("PLSR::EXTRACT::invalid numWorkers: %d", numWorkers))
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
			} else if arg == "--codec" && i+1 < n {
				takingFiles = false
				codec = arguments[i+1]
				i++
			} else if arg == "--preset" && i+1 < n {
				takingFiles = false
				switch arguments[i+1] {
				case "balanced":
					preset = archive.Default
				case "max":
					preset = archive.BestCompression
				case "fast":
					preset = archive.BestSpeed
				default:
					log.Fatal(fmt.Errorf("PLSR::CREATE::invalid preset value: %s", arguments[i+1]))
				}

				i++
			} else if arg == "--chunk-size" && i+1 < n {
				takingFiles = false
				var err error
				chunkSize, err = strconv.Atoi(arguments[i+1])

				if chunkSize < 0 {
					log.Fatal(fmt.Errorf("PLSR::CREATE::invalid chunk size: %d", chunkSize))
				}

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

		if err := create(archivePath, files, dirs, outputDir, version, numWorkers, uint32(chunkSize), codec, preset); err != nil {
			log.Fatal(fmt.Errorf("PLSR::%w", err))
		}

	case "info":

		if len(arguments) != 2 {
			handle_invalid("info")
			return
		}

		info, err := getArchiveInfo(arguments[1])
		if err != nil {
			log.Fatal(fmt.Errorf("PLSR::%w", err))
		}

		printArchiveInfo(info)

	default:
		handle_invalid(arguments[0])
	}
}
