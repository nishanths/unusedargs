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
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/gcexportdata"
)

const help = `Usage:
  unusedarg [flags] # runs on package in current directory
  unusedarg [flags] [packages]
  unusedarg [flags] [directories] # where a '/...' suffix includes all sub-directories
  unusedarg [flags] [files]

Flags:
  -ignore <type>   Don't complain about the specified type; can be repeated to specify  
                   multiple types to ignore.
  -h, -help        Print usage information and exit.
`

func usage() {
	fmt.Fprint(os.Stderr, help)
	os.Exit(2)
}

var (
	ignoreTypes = make(multiFlag)
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("unusedarg: ")

	flag.Var(&ignoreTypes, "ignore", "types to ignore")
	flag.Usage = usage
	flag.Parse() // needed for -help

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
	findUnused(contents, ignoreTypes)
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

func findUnused(files map[string][]byte, ignoreTypes map[string]struct{}) error {
	fset := token.NewFileSet()
	uniquePkgNames := make(map[string]struct{})
	var parsedFiles []file

	for path, content := range files {
		f, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return err
		}
		uniquePkgNames[f.Name.Name] = struct{}{}
		parsedFiles = append(parsedFiles, file{
			file: f,
			path: path,
			pkg:  f.Name.Name,
			// info is set below
			generated: isGenerated(content),
		})
	}

	// Sort to ensure same order across runs.
	var sortedPkgNames []string
	for n := range uniquePkgNames {
		sortedPkgNames = append(sortedPkgNames, n)
	}
	sort.Slice(sortedPkgNames, func(i, j int) bool {
		return sortedPkgNames[i] < sortedPkgNames[j]
	})

	pkgInfos := make(map[string]*types.Info)

	// NOTE: We don't care if there's more than one package in the directory
	// path. We'll be type checking per package anyway.
	importer := gcexportdata.NewImporter(fset, make(map[string]*types.Package))
	config := &types.Config{
		Error:    func(error) {}, // keep going on error
		Importer: importer,
	}

	// Check each package.
	for _, pkg := range sortedPkgNames {
		var astFiles []*ast.File
		for _, f := range parsedFiles {
			if f.pkg == pkg {
				astFiles = append(astFiles, f.file)
			}
		}
		info := &types.Info{
			Types:  make(map[ast.Expr]types.TypeAndValue),
			Defs:   make(map[*ast.Ident]types.Object),
			Uses:   make(map[*ast.Ident]types.Object),
			Scopes: make(map[ast.Node]*types.Scope),
		}
		_, err := config.Check("", fset, astFiles, info)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to type check package %s: results may be partial: %s", pkg, err)
		}

		// Record the info for the package.
		pkgInfos[pkg] = info

		// Set the info field on file.
		for i := range parsedFiles {
			if parsedFiles[i].pkg == pkg {
				parsedFiles[i].info = info
			}
		}
	}

	// Map from function input's position to the function input target that needs
	// to be satisfied. token.Position is valid to use as a map key here
	// because of its uniqueness across files. However, it is not unique
	// across packages.
	//
	// Invariant: len(toSatisfy) == number of inputs across all functions,
	// excluding filtered out inputs.
	//
	// The map is structured this way since we need to be able to:
	//   1. Lookup the funcInput for a given position quickly
	//   2. Iterate over funcInputs to see which ones haven't been satisfied; and
	//   3. For the unsatisfied funcInputs, print the function's name and position.
	type targetsInPackage map[token.Position]target

	// Map from package name to targets for the package.
	allTargets := make(map[string]targetsInPackage)

	matches := matchesTypes(ignoreTypes)

	for _, f := range parsedFiles {
		// Don't look at generated files.
		if f.generated {
			continue
		}

		// Walk looking for functions.
		ast.Walk(walker(func(n ast.Node) {
			var inp []funcInput
			var funcPosition token.Position
			var funcName string

			// Functions can either be function declarations (top-level)
			// or function literals.
			switch c := n.(type) {
			case *ast.FuncDecl:
				inp = inputs(c.Recv, c.Type.Params)
				funcPosition = fset.Position(c.Pos())
				funcName = c.Name.Name
			case *ast.FuncLit:
				inp = inputs(nil, c.Type.Params)
				funcPosition = fset.Position(c.Pos())
			}

			for _, in := range inp {
				if isBlankIdent(in.ident, in.field, f.info) {
					continue
				}
				if matches(in.ident, in.field, f.info) {
					continue
				}
				if allTargets[f.pkg] == nil {
					allTargets[f.pkg] = make(targetsInPackage)
				}
				allTargets[f.pkg][fset.Position(in.pos)] = target{
					input:        in,
					funcPosition: funcPosition,
					funcName:     funcName,
				}
			}
		}), f.file)
	}

	for _, pkg := range sortedPkgNames {
		targets := allTargets[pkg] // targets for this package.

		// Mark inputs as satisfied
		for _, obj := range pkgInfos[pkg].Uses {
			in, ok := targets[fset.Position(obj.Pos())]
			if !ok {
				continue // not a use we care about
			}
			in.satisfiedBy++
			targets[fset.Position(obj.Pos())] = in
		}

		// Print the unsatisfied inputs.
		// First, sort by the filename, line number, and column so that ordering is the same
		// across runs.
		var sortedTargets []target
		for _, v := range targets {
			sortedTargets = append(sortedTargets, v)
		}

		sort.Slice(sortedTargets, func(i, j int) bool {
			a, b := sortedTargets[i], sortedTargets[j]
			if a.funcPosition.Filename < b.funcPosition.Filename {
				return true
			}
			if a.funcPosition.Filename > b.funcPosition.Filename {
				return false
			}
			if a.funcPosition.Line < b.funcPosition.Line {
				return true
			}
			if a.funcPosition.Line > b.funcPosition.Line {
				return false
			}
			return a.funcPosition.Column < b.funcPosition.Column
		})

		for _, t := range sortedTargets {
			name := t.funcName
			if name == "" {
				name = "function literal"
			}
			if t.satisfiedBy < 1 {
				fmt.Printf("%s: %s has unused %s %s\n", t.funcPosition, name, t.input.kind, t.input.ident)
			}
		}
	}

	return nil
}

type target struct {
	input        funcInput
	funcPosition token.Position
	funcName     string
	satisfiedBy  int
}

type file struct {
	file      *ast.File
	path      string
	pkg       string
	info      *types.Info
	generated bool
}

const (
	kindReceiver = "receiver"
	kindParam    = "param"
)

type funcInput struct {
	ident *ast.Ident
	field *ast.Field
	kind  string
	pos   token.Pos
}

func inputs(recv, params *ast.FieldList) []funcInput {
	var inp []funcInput
	if recv != nil {
		for _, field := range recv.List {
			for _, name := range field.Names {
				inp = append(inp, funcInput{
					ident: name,
					field: field,
					kind:  kindReceiver,
					pos:   name.NamePos,
				})
			}
		}
	}
	for _, field := range params.List {
		// Params without names such as func foo(int) are automatically
		// ignored since Names will be empty.
		for _, name := range field.Names {
			inp = append(inp, funcInput{
				ident: name,
				field: field,
				kind:  kindParam,
				pos:   name.NamePos,
			})
		}
	}
	return inp
}

type filterFunc func(*ast.Ident, *ast.Field, *types.Info) bool

// Filter functions.
var (
	isBlankIdent = func(name *ast.Ident, _ *ast.Field, _ *types.Info) bool {
		return name.Name == "_"
	}
	matchesTypes = func(set map[string]struct{}) filterFunc {
		return func(_ *ast.Ident, field *ast.Field, info *types.Info) bool {
			// TODO: make less hacky?
			t := info.TypeOf(field.Type) // will be nil for *ast.Ellipsis, etc.
			if t != nil {
				_, ok := set[t.String()]
				return ok
			}
			return false
		}
	}
)

// walker makes a function implement ast.Visitor.
type walker func(ast.Node)

func (w walker) Visit(node ast.Node) ast.Visitor {
	w(node)
	return w
}
