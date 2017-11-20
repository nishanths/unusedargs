// Packages usages finds the usage sites of the all the receivers and parameters
// of functions in a set of Go source files. The API isn't great; it's suited
// for use by the unusedargs command.
package usages

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/gcexportdata"
)

// Function input kinds.
const (
	FuncReceiver string = "receiver"
	FuncParam           = "param"
)

// Result is the uses for the receiver/param of a function.
type Result struct {
	Ident    *ast.Ident     // ident for the receiver/param
	Field    *ast.Field     // field for the receiver/param
	Kind     string         // either FuncReceiver or FuncParam
	Position token.Position // position of receiver/param

	Uses []*ast.Ident // uses of this variable

	FuncPosition token.Position // position of function
	FuncName     string         // name of function, or empty string if function literal
}

type file struct {
	file *ast.File
	pkg  string
	info *types.Info
}

type target struct {
	funcInput    funcInput
	funcPosition token.Position
	funcName     string
	uses         []*ast.Ident
}

// Find finds the usages of the receivers and params of functions
// in the supplied files. Files is a map from the file's path to its contents.
// The results is a map from the package name to the usage results.
// typeInfo is a map from the package name to the type info for the package.
// If there was an error type checking a package, it is returned via warns.
//
// The results are presented in file order (i.e. sorted lexicographically
// by filename, then line number, then column number).
//
//   Invariant: len(results) == number of packages.
//   Invariant: len(results[key]) == number of receivers/params, except blank
//              identifiers or unnamed receivers/params.
func Find(files map[string][]byte) (results map[string][]Result, typeInfo map[string]*types.Info,
	warns map[string][]error, err error) {
	fset := token.NewFileSet()
	uniquePkgNames := make(map[string]struct{})
	var parsedFiles []file

	// Parse the files; determine the packages that are present.
	for path, content := range files {
		f, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return nil, nil, warns, err
		}
		uniquePkgNames[f.Name.Name] = struct{}{}
		parsedFiles = append(parsedFiles, file{
			file: f,
			pkg:  f.Name.Name,
			// info is set below
		})
	}

	// NOTE: We don't care if there's more than one package in the directory
	// path. We'll be type checking per package anyway.
	// If the type checker errors out on the multiple packages, we'll warn
	// them, but it shouldn't affect what we're doing.
	importer := gcexportdata.NewImporter(fset, make(map[string]*types.Package))
	config := &types.Config{
		Error:    func(error) {}, // keep going on error
		Importer: importer,
	}

	// Map from package to type info for that package.
	pkgInfos := make(map[string]*types.Info)
	warns = make(map[string][]error)

	// Check each package, and record the type info.
	for pkg := range uniquePkgNames {
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
			warns[pkg] = append(warns[pkg], err)
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

	// Map from position of the function param/receiver to the target that needs
	// to be satisfied. token.Position is valid to use as a map key here
	// because of its uniqueness across files. However, it is not unique
	// across packages.
	//
	// The map is structured this way since we need to be able to:
	//   1. Lookup the target for a given position quickly
	//   2. Iterate over targets to see which ones haven't been satisfied
	type targetsForPackage map[token.Position]target

	// Map from package name to targets for the package.
	allTargets := make(map[string]targetsForPackage)

	// Walk the parsed files; looking for functions.
	for _, f := range parsedFiles {
		ast.Inspect(f.file, func(n ast.Node) bool {
			var inp []funcInput
			var funcPosition token.Position
			var funcName string

			// Functions can either be function declarations (top-level)
			// or function literals.
			switch c := n.(type) {
			case *ast.FuncDecl:
				inp = inputs(c.Recv, c.Type.Params)
				funcPosition = fset.Position(c.Name.Pos())
				funcName = c.Name.Name
			case *ast.FuncLit:
				inp = inputs(nil, c.Type.Params)
				funcPosition = fset.Position(c.Pos())
			}

			// Add the functions inputs to the map of all
			// the targets that need to be satisified.
			for _, in := range inp {
				if isBlankIdent(in.ident) {
					continue
				}
				if allTargets[f.pkg] == nil {
					allTargets[f.pkg] = make(targetsForPackage)
				}
				allTargets[f.pkg][fset.Position(in.pos)] = target{
					funcInput:    in,
					funcPosition: funcPosition,
					funcName:     funcName,
					// uses filled in below
				}
			}

			return true
		})
	}

	// Make results for each package.
	results = make(map[string][]Result)
	for pkg := range uniquePkgNames {
		targets := allTargets[pkg]
		info := pkgInfos[pkg]
		results[pkg] = makeResult(targets, info, fset)
	}
	return results, pkgInfos, warns, nil
}

// makeResult computes results for a package.
func makeResult(targets map[token.Position]target, info *types.Info, fset *token.FileSet) []Result {
	var r []Result

	// Mark function receiver/parameter as satisfied.
	for id, obj := range info.Uses {
		t, ok := targets[fset.Position(obj.Pos())]
		if !ok {
			continue // not a use we care about
		}
		t.uses = append(t.uses, id)
		targets[fset.Position(obj.Pos())] = t
	}

	// Now we can compute the usage results.
	// But first, sort by the filename, line number, and column.
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
		r = append(r, Result{
			Ident:        t.funcInput.ident,
			Field:        t.funcInput.field,
			Kind:         t.funcInput.kind,
			Uses:         t.uses,
			Position:     fset.Position(t.funcInput.pos),
			FuncPosition: t.funcPosition,
			FuncName:     t.funcName,
		})
	}

	return r
}

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
					kind:  FuncReceiver,
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
				kind:  FuncParam,
				pos:   name.NamePos,
			})
		}
	}
	return inp
}

func isBlankIdent(name *ast.Ident) bool {
	return name.Name == "_"
}
