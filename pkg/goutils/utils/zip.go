package utils

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Unzip will decompress a zipped content, moving all files and folders
// within the zip content (parameter 1) to an output directory (parameter 2).
func Unzip(zippedData []byte, dest string) ([]string, error) {
	return UnzipStream(bytes.NewReader(zippedData), int64(len(zippedData)), dest)
}

func UnzipStream(reader io.ReaderAt, size int64, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.NewReader(reader, size)
	if err != nil {
		return filenames, err
	}

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, f.Name)

		if f.FileInfo().IsDir() {
			// Make Folder
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return nil, err
			}
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}
