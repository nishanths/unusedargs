package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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
			handleFiles(results)
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
			fmt.Fprintf(os.Stderr, "skipping file: %s", err)
			continue
		}
		contents[name] = b
	}
	findUnused(contents)
}

func findUnused(files map[string][]byte) error {
	fset := token.NewFileSet()
	parsedFiles := make(map[string]file)

	var firstSeenPkgName string
	for filename, content := range files {
		if isGenerated(content) {
			continue // skip generated files; assume they know what they're doing
		}
		f, err := parser.ParseFile(fset, filename, content, 0)
		if err != nil {
			return err
		}
		if firstSeenPkgName == "" {
			firstSeenPkgName = f.Name.Name
		}
		if firstSeenPkgName != f.Name.Name {
			return fmt.Errorf("%s file %s is not in package %s", f.Name.Name, firstSeenPkgName)
		}
		parsedFiles[filename] = file{f}
	}

	for _, f := range parsedFiles {
		f.check()
	}
	return nil
}

type file struct {
	file *ast.File
}

func (f file) check() {
	ast.Walk(walker(func(n ast.Node) {
		switch c := n.(type) {
		case *ast.FuncDecl:
			function{
				recv:   c.Recv,
				name:   c.Name,
				params: c.Type.Params,
				body:   c.Body,
			}.check()
		case *ast.FuncLit:
			function{
				params: c.Type.Params,
				body:   c.Body,
			}.check()
		}
	}), f.file)
}

type function struct {
	recv   *ast.FieldList // always nil for FuncLit; nil for FuncDecl if no receiver
	name   *ast.Ident     // always nil for FuncLit
	params *ast.FieldList // incoming parameters
	body   *ast.BlockStmt // function body
}

const (
	kindRecv  = "receiver"
	kindParam = "param"
)

type arg struct {
	name string
	kind string
}

func (f function) args() []arg {
	var names []arg
	if f.recv != nil {
		for _, field := range f.recv.List {
			for _, name := range field.Names {
				names = append(names, arg{
					name: name.Name,
					kind: kindRecv,
				})
			}
		}
	}
	for _, field := range f.params.List {
		for _, name := range field.Names {
			names = append(names, arg{
				name: name.Name,
				kind: kindParam,
			})
		}
	}
	return names
}

func (f function) check() {
	fmt.Println(f.name, f.args())
	ast.Walk(walker(func(n ast.Node) {
		fmt.Println(n)
	}), f.body)
	fmt.Println()
}

// walker makes a function implement ast.Visitor.
type walker func(ast.Node)

func (w walker) Visit(node ast.Node) ast.Visitor {
	w(node)
	return w
}

var (
	genHdr = []byte("// Code generated ")
	genFtr = []byte(" DO NOT EDIT.")
)

// isGenerated reports whether the source file is generated code
// according to the rules from https://golang.org/s/generatedcode.
func isGenerated(src []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		b := sc.Bytes()
		if bytes.HasPrefix(b, genHdr) && bytes.HasSuffix(b, genFtr) && len(b) >= len(genHdr)+len(genFtr) {
			return true
		}
	}
	return false
}
