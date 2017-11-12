package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
)

const help = `Usage of unusedarg:
	unusedarg # runs on package in current directory
	unusedarg [packages]
	unusedarg [directories] # where a '/...' suffix includes all sub-directories
	unusedarg [files] # all must belong to a single package
`

func usage() {
	fmt.Fprint(os.Stderr, help)
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("unusedarg: ")

	flag.Usage = usage
	flag.Parse() // needed for -help

	args := flag.Args()

	if len(args) == 0 {
		handleDir(".")
	} else {
		dirsRun, filesRun, pkgsRun, results := classifyArgs(args)
		if dirsRun+filesRun+pkgsRun != 1 {
			usage()
		}
		switch {
		case dirsRun == 1:
			for _, dir := range results {
				handleDir(dir)
			}
		case filesRun == 1:
			handleFiles(results...)
		case pkgsRun == 1:
			for _, pkg := range results {
				handlePkg(pkg)
			}
		default:
			panic("code bug")
		}
	}
}

func isIgnorable(err error) bool {
	if _, nogo := err.(*build.NoGoError); nogo {
		// Don't complain if the failure is due to no Go source files.
		return true
	}
	return false
}

func handleDir(p string) {
	pkg, err := build.ImportDir(p, 0)
	if err != nil {
		if isIgnorable(err) {
			os.Exit(0)
		}
		log.Fatal(err)
	}
	handleImportedPkg(pkg)
}

func handleFiles(p ...string) {
}

func handlePkg(p string) {
	pkg, err := build.Import(p, ".", 0)
	if err != nil {
		if isIgnorable(err) {
			os.Exit(0)
		}
		log.Fatal(err)
	}
	handleImportedPkg(pkg)
}

func handleImportedPkg(pkg *build.Package) {
}
