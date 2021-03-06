package main

import (
	"os"
	"strings"
)

// dirs, files, and pkgs indicate whether args is applied to
// directory, file or package targets.
//
// Copied from main() in:
// https://github.com/golang/lint/blob/6aaf7c34af0f4c36a57e0c429bace4d706d8e931/golint/main.go
func classifyArgs(args []string) (dirs, files, pkgs int, results []string) {
	for _, arg := range args {
		if strings.HasSuffix(arg, "/...") && isDir(arg[:len(arg)-len("/...")]) {
			dirs = 1
			for _, dirname := range allPackagesInFS(arg) {
				results = append(results, dirname)
			}
		} else if isDir(arg) {
			dirs = 1
			results = append(results, arg)
		} else if exists(arg) {
			files = 1
			results = append(results, arg)
		} else {
			pkgs = 1
			results = append(results, arg)
		}
	}
	return
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
