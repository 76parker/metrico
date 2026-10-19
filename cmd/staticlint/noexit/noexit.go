// Package noexit запрещает прямой вызов os.Exit из функции main пакета main.
package noexit

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// New создаёт анализатор, пропускающий тела вложенных анонимных функций.
func New() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "noexit",
		Doc:  "forbid direct calls to os.Exit in the main function of package main",
		Run:  run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Name.Name != "main" || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, ok := node.(*ast.FuncLit); ok {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				var identifier *ast.Ident
				switch called := ast.Unparen(call.Fun).(type) {
				case *ast.SelectorExpr:
					identifier = called.Sel
				case *ast.Ident:
					identifier = called
				}
				if identifier == nil {
					return true
				}
				function, ok := pass.TypesInfo.Uses[identifier].(*types.Func)
				if ok && function.Pkg() != nil && function.Pkg().Path() == "os" && function.Name() == "Exit" {
					pass.Reportf(call.Pos(), "direct call to os.Exit in main is forbidden")
				}
				return true
			})
		}
	}
	return nil, nil
}
