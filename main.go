package main

import (
	"flag"
	"fmt"
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
			panic("CODE BUG!")
		}
	}
}

func handleDir(p string) {}

func handleFiles(p ...string) {}

func handlePkg(p string) {}
