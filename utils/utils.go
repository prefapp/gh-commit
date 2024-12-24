package utils

import (
	"os"
)

func isFile(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		// handle the error if needed
		return false
	}
	return !fileInfo.IsDir()
}

func ListFiles(path string, ignoredFolders []string) []string {
	var files []string

	// if path is a file just return the path
	if isFile(path) {
		return []string{path}
	}
	dir, err := os.Open(path)
	if err != nil {
		// handle the error if needed
		return nil
	}
	defer dir.Close()

	// read the contents of the directory
	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		// handle the error if needed
		return nil
	}

	// loop through the fileInfos
	for _, fi := range fileInfos {
		// check if the folder is in the ignored list
		ignored := false
		for _, ignoredFolder := range ignoredFolders {
			if fi.Name() == ignoredFolder {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		if fi.IsDir() {
			// if the file is a directory
			files = append(files, ListFiles(path+string(os.PathSeparator)+fi.Name(), ignoredFolders)...)
		} else {
			// if the file is a regular file
			files = append(files, path+string(os.PathSeparator)+fi.Name())
		}
	}
	return files
}

func FileExistsInList(files []string, target string) bool {
	for _, file := range files {
		if file == target {
			return true
		}
	}
	return false
}
