//go:build ignore

// Instrument an isolated source copy. Never run against the working source tree.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type site struct {
	ID         string   `json:"id"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Kind       string   `json:"kind"`
	Expression string   `json:"expression"`
	Outcomes   []string `json:"outcomes"`
}

var sites []site
var sets = token.NewFileSet()
var sequence int

func str(s string) ast.Expr { return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)} }
func call(name string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("_branchProbe"), Sel: ast.NewIdent(name)}, Args: args}
}
func hit(id, outcome string) ast.Stmt { return &ast.ExprStmt{X: call("Hit", str(id), str(outcome))} }
func add(path, kind string, node ast.Node, expr ast.Node, outcomes []string) string {
	position := sets.Position(node.Pos())
	id := fmt.Sprintf("%s:%d:%d:%s", path, position.Line, position.Column, kind)
	var text bytes.Buffer
	if expr != nil {
		format.Node(&text, sets, expr)
	}
	sites = append(sites, site{id, path, position.Line, kind, text.String(), outcomes})
	return id
}
func ranges(list []ast.Stmt, path string) []ast.Stmt {
	out := make([]ast.Stmt, 0, len(list))
	for _, statement := range list {
		var r *ast.RangeStmt
		switch s := statement.(type) {
		case *ast.RangeStmt:
			r = s
		case *ast.LabeledStmt:
			r, _ = s.Stmt.(*ast.RangeStmt)
		}
		if r == nil {
			out = append(out, statement)
			continue
		}
		sequence++
		name := fmt.Sprintf("_branchRangeSeen%d", sequence)
		id := add(path, "range", r, r.X, []string{"empty", "nonempty"})
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent("false")}})
		r.Body.List = append([]ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}}, hit(id, "nonempty")}, r.Body.List...)
		out = append(out, statement, &ast.IfStmt{Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(name)}, Body: &ast.BlockStmt{List: []ast.Stmt{hit(id, "empty")}}})
	}
	return out
}
func instrument(path, destination string) error {
	file, err := parser.ParseFile(sets, destination, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	before := len(sites)
	changed := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IfStmt:
			id := add(path, "if", n, n.Cond, []string{"false", "true"})
			n.Cond = call("Bool", str(id), &ast.CallExpr{Fun: ast.NewIdent("bool"), Args: []ast.Expr{n.Cond}})
		case *ast.ForStmt:
			if n.Cond != nil {
				id := add(path, "for", n, n.Cond, []string{"false", "true"})
				n.Cond = call("Bool", str(id), &ast.CallExpr{Fun: ast.NewIdent("bool"), Args: []ast.Expr{n.Cond}})
			}
		case *ast.SwitchStmt:
			id := add(path, "switch", n, n.Tag, nil)
			index := len(sites) - 1
			hasDefault := false
			for i, body := range n.Body.List {
				clause := body.(*ast.CaseClause)
				outcome := fmt.Sprintf("case%d", i)
				if clause.List == nil {
					outcome = "default"
					hasDefault = true
				}
				sites[index].Outcomes = append(sites[index].Outcomes, outcome)
				clause.Body = append([]ast.Stmt{hit(id, outcome)}, clause.Body...)
			}
			if !hasDefault {
				sites[index].Outcomes = append(sites[index].Outcomes, "default")
				n.Body.List = append(n.Body.List, &ast.CaseClause{Body: []ast.Stmt{hit(id, "default")}})
			}
		case *ast.TypeSwitchStmt:
			id := add(path, "type_switch", n, n.Assign, nil)
			index := len(sites) - 1
			hasDefault := false
			for i, body := range n.Body.List {
				clause := body.(*ast.CaseClause)
				outcome := fmt.Sprintf("case%d", i)
				if clause.List == nil {
					outcome = "default"
					hasDefault = true
				}
				sites[index].Outcomes = append(sites[index].Outcomes, outcome)
				clause.Body = append([]ast.Stmt{hit(id, outcome)}, clause.Body...)
			}
			if !hasDefault {
				sites[index].Outcomes = append(sites[index].Outcomes, "default")
				n.Body.List = append(n.Body.List, &ast.CaseClause{Body: []ast.Stmt{hit(id, "default")}})
			}
		case *ast.SelectStmt:
			id := add(path, "select", n, nil, nil)
			index := len(sites) - 1
			for i, body := range n.Body.List {
				clause := body.(*ast.CommClause)
				outcome := fmt.Sprintf("case%d", i)
				if clause.Comm == nil {
					outcome = "default"
				}
				sites[index].Outcomes = append(sites[index].Outcomes, outcome)
				clause.Body = append([]ast.Stmt{hit(id, outcome)}, clause.Body...)
			}
		case *ast.CallExpr:
			if selector, ok := n.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "os" && selector.Sel.Name == "Exit" {
					n.Fun = &ast.SelectorExpr{X: ast.NewIdent("_branchProbe"), Sel: ast.NewIdent("Exit")}
					changed = true
				}
			}
		}
		return true
	})
	// A separate pass keeps the generated empty-range check out of the inventory.
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BlockStmt:
			n.List = ranges(n.List, path)
		case *ast.CaseClause:
			n.Body = ranges(n.Body, path)
		case *ast.CommClause:
			n.Body = ranges(n.Body, path)
		}
		return true
	})
	if file.Name.Name == "main" {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
				fn.Body.List = append([]ast.Stmt{&ast.DeferStmt{Call: call("Flush")}}, fn.Body.List...)
				changed = true
			}
		}
	}
	if len(sites) == before && !changed {
		return nil
	}
	declaration := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{&ast.ImportSpec{Name: ast.NewIdent("_branchProbe"), Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("example.com/go-block-lattice/internal/branchprobe")}}}}
	file.Decls = append([]ast.Decl{declaration}, file.Decls...)
	var output bytes.Buffer
	if err = format.Node(&output, sets, file); err != nil {
		return err
	}
	return os.WriteFile(destination, output.Bytes(), 0600)
}

const probe = `package branchprobe
import("encoding/json";"os";"path/filepath";"sync")
var mu sync.Mutex
var hits=map[string]map[string]uint64{}
var directory=os.Getenv("BL_BRANCH_OUT")
func Hit(id,outcome string){if directory==""{return};mu.Lock();if hits[id]==nil{hits[id]=map[string]uint64{}};hits[id][outcome]++;mu.Unlock()}
func Bool(id string,value bool)bool{if value{Hit(id,"true")}else{Hit(id,"false")};return value}
func Flush(){if directory==""{return};mu.Lock();defer mu.Unlock();f,err:=os.CreateTemp(filepath.Clean(directory),"decisions-*.json");if err!=nil{panic(err)};if err=json.NewEncoder(f).Encode(hits);err!=nil{f.Close();panic(err)};if err=f.Close();err!=nil{panic(err)}}
func Exit(code int){Flush();os.Exit(code)}
`

func main() {
	if len(os.Args) != 3 {
		panic("usage: instrument COPY INVENTORY")
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if root == cwd {
		panic("refusing to instrument the working source tree")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		panic("refusing to instrument a Git checkout")
	}
	for _, base := range []string{"internal", "cmd"} {
		err = filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			return instrument(filepath.ToSlash(relative), path)
		})
		if err != nil {
			panic(err)
		}
	}
	helper := filepath.Join(root, "internal", "branchprobe")
	if err = os.MkdirAll(helper, 0700); err != nil {
		panic(err)
	}
	if err = os.WriteFile(filepath.Join(helper, "probe.go"), []byte(probe), 0600); err != nil {
		panic(err)
	}
	for _, base := range []string{"internal", "cmd"} {
		entries, err := os.ReadDir(filepath.Join(root, base))
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "branchprobe" {
				continue
			}
			dir := filepath.Join(root, base, entry.Name())
			files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
			if err != nil {
				panic(err)
			}
			if len(files) == 0 {
				continue
			}
			var packageName string
			for _, path := range files {
				f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
				if err != nil {
					panic(err)
				}
				if !strings.HasSuffix(f.Name.Name, "_test") {
					packageName = f.Name.Name
					break
				}
			}
			if packageName == "" {
				continue
			}
			content := fmt.Sprintf("package %s\nimport(\"os\";\"testing\";_branchProbe \"example.com/go-block-lattice/internal/branchprobe\")\nfunc TestMain(m *testing.M){code:=m.Run();_branchProbe.Flush();os.Exit(code)}\n", packageName)
			if err = os.WriteFile(filepath.Join(dir, "branch_probe_test.go"), []byte(content), 0600); err != nil {
				panic(err)
			}
		}
	}
	data, err := json.MarshalIndent(sites, "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(os.Args[2], data, 0600); err != nil {
		panic(err)
	}
	fmt.Printf("Instrumented %d decision sites in isolated copy\n", len(sites))
}
