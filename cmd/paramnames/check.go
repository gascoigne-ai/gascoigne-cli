package main

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Violation is one one-letter parameter name.
type Violation struct {
	File    string
	Line    int
	Name    string
	Package string
}

// PackageCount is the number of violations in one package directory.
type PackageCount struct {
	Package string
	Count   int
}

// testingTypes are the testing types whose t/b parameter names are allowed.
var testingTypes = []string{"T", "B", "F", "TB"}

// testingNames are the one-letter names Go style keeps for testing types.
var testingNames = []string{"t", "b"}

// check parses src and returns its one-letter parameters. The package is the
// file's directory, so a _test package folds into the package it tests.
func check(name string, src []byte) ([]Violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	testing := testingImportName(file)
	var violations []Violation
	ast.Inspect(file, func(node ast.Node) bool {
		funcType, ok := node.(*ast.FuncType)
		if !ok || funcType.Params == nil {
			return true
		}
		for _, field := range funcType.Params.List {
			for _, ident := range field.Names {
				if !oneLetter(ident.Name) || (testingParam(ident.Name, field.Type, testing)) {
					continue
				}
				violations = append(violations, Violation{
					File:    name,
					Line:    fset.Position(ident.Pos()).Line,
					Name:    ident.Name,
					Package: filepath.ToSlash(filepath.Dir(name)),
				})
			}
		}
		return true
	})
	return violations, nil
}

func oneLetter(name string) bool {
	return name != "_" && utf8.RuneCountInString(name) == 1
}

// testingImportName is the local name of the "testing" import, or "" when the
// file has none (a dot or blank import names nothing usable).
func testingImportName(file *ast.File) string {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "testing" {
			continue
		}
		if spec.Name == nil {
			return "testing"
		}
		if spec.Name.Name == "_" || spec.Name.Name == "." {
			return ""
		}
		return spec.Name.Name
	}
	return ""
}

// testingParam reports whether name of type expr is an allowed t/b of a testing type.
func testingParam(name string, expr ast.Expr, testing string) bool {
	if testing == "" || !slices.Contains(testingNames, name) {
		return false
	}
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == testing && slices.Contains(testingTypes, selector.Sel.Name)
}

// counts totals violations per package, sorted by package.
func counts(violations []Violation) []PackageCount {
	perPackage := map[string]int{}
	for _, violation := range violations {
		perPackage[violation.Package]++
	}
	result := make([]PackageCount, 0, len(perPackage))
	for pkg, count := range perPackage {
		result = append(result, PackageCount{pkg, count})
	}
	slices.SortFunc(result, func(left, right PackageCount) int {
		return cmp.Compare(left.Package, right.Package)
	})
	return result
}
