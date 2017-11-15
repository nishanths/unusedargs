package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"

	usages "github.com/nishanths/unusedarg/usages"
)

const help = `Usage:
  unusedarg [flags] # runs on package in current directory
  unusedarg [flags] [packages]
  unusedarg [flags] [directories] # where a '/...' suffix includes all sub-directories
  unusedarg [flags] [files]

Flags:
  -ignore <type>    Don't complain about the specified type; can be repeated to specify  
                    multiple types to ignore.
  -h, -help         Print usage information and exit.
  -format <format>  Output format; one of text,json (default: text).

Unusedarg reports unused receivers and paramters in functions.

`

func usage() {
	fmt.Fprint(os.Stderr, help)
	os.Exit(2)
}

// mulitFlag can be a used as a flag.Var.
type multiFlag map[string]struct{}

func (m multiFlag) String() string {
	var buf bytes.Buffer
	i := 0
	for k := range m {
		buf.WriteString(k)
		if i != len(m) {
			buf.WriteString(", ")
		}
	}
	return buf.String()
}

func (m multiFlag) Set(x string) error {
	m[x] = struct{}{}
	return nil
}

var ignoreTypes = make(multiFlag)

func main() {
	log.SetFlags(0)
	log.SetPrefix("unusedarg: ")

	flag.Var(&ignoreTypes, "ignore", "types to ignore")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()

	if len(args) == 0 {
		handleDir(".")
	} else {
		dirsRun, filesRun, pkgsRun, results := classifyArgs(args)
		// TODO(nishanth): This kind of return value feels gross.
		if dirsRun+filesRun+pkgsRun != 1 {
			usage()
		}
		switch {
		case dirsRun == 1:
			for _, dir := range results {
				handleDir(dir)
			}
		case filesRun == 1:
			handleFiles(results)
		case pkgsRun == 1:
			for _, pkg := range importPaths(results) {
				handlePkg(pkg)
			}
		default:
			// cannot happen
			panic("code bug: expected one dirsRun|filesRun|pkgsRun to be 1")
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
		if !isIgnorable(err) {
			log.Fatal(err)
		}
		return
	}
	handleImportedPkg(pkg)
}

func handlePkg(p string) {
	pkg, err := build.Import(p, ".", 0)
	if err != nil {
		if !isIgnorable(err) {
			log.Fatal(err)
		}
		return
	}
	handleImportedPkg(pkg)
}

func handleImportedPkg(pkg *build.Package) {
	var files []string
	files = append(files, pkg.GoFiles...)
	files = append(files, pkg.CgoFiles...)
	files = append(files, pkg.TestGoFiles...)
	files = append(files, pkg.XTestGoFiles...)
	if pkg.Dir != "." {
		for i, f := range files {
			files[i] = filepath.Join(pkg.Dir, f)
		}
	}
	handleFiles(files)
}

func handleFiles(files []string) {
	contents := make(map[string][]byte)
	for _, name := range files {
		b, err := ioutil.ReadFile(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping: %s", err)
			continue
		}
		contents[name] = b
	}

	results, typeInfo, warns, err := usages.Find(contents)
	if err != nil {
		log.Fatal(err)
	}

	// Sort warnings.
	var warnsOrder []string
	for pkg := range warns {
		warnsOrder = append(warnsOrder, pkg)
	}
	sort.Slice(warnsOrder, func(i, j int) bool {
		return warnsOrder[i] < warnsOrder[j]
	})

	// Print warnings (once per package).
	printedWarns := make(map[string]bool)
	for _, pkg := range warnsOrder {
		if printedWarns[pkg] {
			continue // already printed
		}
		printedWarns[pkg] = true
		fmt.Fprintf(os.Stderr, "failed to type check package %s: results may be partial")
	}

	// Sort results packages.
	var resultsOrder []string
	for pkg := range results {
		resultsOrder = append(resultsOrder, pkg)
	}
	sort.Slice(resultsOrder, func(i, j int) bool {
		return resultsOrder[i] < resultsOrder[j]
	})

	// Print results.
	for _, pkg := range resultsOrder {
		for _, r := range results[pkg] {
			if len(r.Uses) > 0 {
				continue // has uses
			}
			t := typeInfo[pkg].TypeOf(r.Field.Type)
			if t != nil {
				_, ok := ignoreTypes[t.String()]
				if ok {
					continue // ignored type
				}
			}
			name := r.FuncName
			if name == "" {
				name = "function literal"
			}
			fmt.Printf("%s: %s has unused %s %s\n", r.FuncPosition, name, r.Kind, r.Ident.Name)
		}
	}
}
