// Command unusedargs reports unused receivers and paramters for functions
// in the specified files, directories, or packages. The exit status is 0 if
// there were no warnings reported, and 1 if there were warnings.
//
//   func authURL(clientID, code int, state string) string {
//       return fmt.Sprintf("https://example.org/?client_id=%d&code=%d", clientID, code)
//   }
//
//   $ unusedargs
//   /home/growl/go/src/code.org/x/main.go:8:1: authURL has unused param state
//
// Ignoring types
//
// To ignore certain types, use the -ignore flag. This is useful for silencing
// reports on types such as context.Context, which typically are introduced
// incrementally through a codebase.
//
//    $ unusedargs -ignore "context.Context" code.org/pkg
//
// Methods satisfying an interface
//
// There are legitimate cases in which a method needs to have unused
// arguments. For example, when trying to conform to the io.Writer interface:
//
//   // BlackHole is an io.Writer that discards everything written to it
//   // without error.
//   type BlackHole struct{}
//
//   func (b *BlackHole) Write(p []byte) (int, error) {
//       return 0, nil
//   }
//
// The Write method here neither uses the receiver nor the incoming parameter.
// An idiomatic way to express that is to omit the identifier or use
// the blank identifier:
//
//   func (*BlackHole) Write(_ []byte) (int, error) {
//       return 0, nil
//   }
//
// which makes it clear to clients that the inputs are not used by the method,
// and also makes the command no longer print a warning.
//
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

	"github.com/nishanths/unusedargs/usages"
)

const help = `Usage:
  unusedarg [flags] # runs on package in current directory
  unusedarg [flags] [packages]
  unusedarg [flags] [directories] # where a '/...' suffix includes all sub-directories
  unusedarg [flags] [files]

Flags:
  -ignore <type>    Don't complain about the specified type; can be repeated to specify  
                    multiple types to ignore. For example: -ignore "context.Context".
  -h, -help         Print usage information and exit.
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

	exitStatus := 0

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
				name = "func"
			}
			exitStatus = 1
			fmt.Printf("%s: %s has unused %s %s\n", r.FuncPosition, name, r.Kind, r.Ident.Name)
		}
	}

	os.Exit(exitStatus)
}
