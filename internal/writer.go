package archive

import (
	"errors"
	"os"
	"strings"
)

var extension string = ".plsr"

type ObjStatus uint8

const (
	Created ObjStatus = 0
	Active  ObjStatus = 2
	Closed  ObjStatus = 3
	Failed  ObjStatus = 4
)

type FileStruct struct {
	Path        string
	ArchivePath string
	Method      CompressionMethod
}

func checkFilename(fileName string) bool {
	if len(fileName) > 255 || strings.Contains(fileName, "/") {
		return false
	}

	return true
}

func dirExists(path string) bool {

	info, err := os.Stat(path)
	if err != nil {

		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return false
	}

	return info.IsDir()
}
