//go:build ignore

// Inject dependency failures only into disposable, non-Git source copies.
package main

import (
	"bytes"
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

type point struct{ ID, Function string }

var points []point

func resultType(call *ast.CallExpr) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	var receiver bytes.Buffer
	format.Node(&receiver, token.NewFileSet(), selector.X)
	name := selector.Sel.Name
	switch receiver.String() {
	case "json":
		if name == "Marshal" || name == "MarshalIndent" {
			return "[]byte"
		}
	case "rand":
		if name == "Read" {
			return "int"
		}
	case "pbkdf2":
		if name == "Key" {
			return "[]byte"
		}
	case "aes":
		if name == "NewCipher" {
			return "cipher.Block"
		}
	case "cipher":
		if name == "NewGCM" {
			return "cipher.AEAD"
		}
	case "p2p":
		if name == "New" {
			return "*p2p.Network"
		}
	case "rpc":
		if name == "New" {
			return "*rpc.Server"
		}
	case "w":
		if name == "PrivateKey" {
			return "ed25519.PrivateKey"
		}
	case "v", "p":
		if name == "Sign" {
			return "error"
		}
	case "os":
		switch name {
		case "Open", "OpenFile", "CreateTemp":
			return "*os.File"
		case "Stat", "Lstat":
			return "os.FileInfo"
		case "ReadFile":
			return "[]byte"
		case "ReadDir":
			return "[]os.DirEntry"
		case "MkdirTemp":
			return "string"
		case "MkdirAll", "Mkdir", "Rename", "Link", "Remove":
			return "error"
		}
	case "filepath":
		if name == "EvalSymlinks" || name == "Abs" {
			return "string"
		}
	case "b":
		if name == "GenerateWork" {
			return "error"
		}
	case "io":
		switch name {
		case "ReadAll":
			return "[]byte"
		case "Copy":
			return "int64"
		}
	case "syscall":
		if name == "Flock" {
			return "error"
		}
	case "counter":
		if name == "CompareAndSwap" {
			return "bool"
		}
	case "srv":
		if name == "Shutdown" {
			return "error"
		}
	case "store":
		if name == "Close" {
			return "error"
		}
	case "f", "file", "j.file", "lock", "d":
		switch name {
		case "Stat":
			return "os.FileInfo"
		case "Seek":
			return "int64"
		case "Write":
			return "int"
		case "Close", "Sync", "Chmod", "Truncate":
			return "error"
		}
	}
	return ""
}

func instrument(path, relative string) error {
	sets := token.NewFileSet()
	file, err := parser.ParseFile(sets, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	before := len(points)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		counts := map[string]int{}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch node.(type) {
			case *ast.DeferStmt, *ast.ExprStmt:
				return false
			}
			if loop, ok := node.(*ast.ForStmt); ok && filepath.Base(relative) == "block.go" && fn.Name.Name == "GenerateWork" {
				if init, ok := loop.Init.(*ast.AssignStmt); ok && len(init.Lhs) == 1 {
					if name, ok := init.Lhs[0].(*ast.Ident); ok && name.Name == "nonce" {
						id := "block.go/GenerateWork/nonce-start"
						points = append(points, point{id, fn.Name.Name})
						expression, err := parser.ParseExpr(`func()uint64{if _auditFail("block.go/GenerateWork/nonce-start"){return math.MaxUint64};return uint64(0)}()`)
						if err != nil {
							panic(err)
						}
						init.Rhs[0] = expression
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if filepath.Base(relative) == "engine.go" && fn.Name.Name == "New" {
				var target bytes.Buffer
				format.Node(&target, sets, call.Fun)
				if target.String() == "cfg.Store.Status" {
					points = append(points, point{"engine.go/New/status-height-limit", fn.Name.Name})
					original := *call
					parsed, err := parser.ParseExpr(`func()ledger.Status{value:=PLACEHOLDER();if _auditFail("engine.go/New/status-height-limit"){value.Height=math.MaxUint64};return value}`)
					if err != nil {
						panic(err)
					}
					lit := parsed.(*ast.FuncLit)
					lit.Body.List[0].(*ast.AssignStmt).Rhs = []ast.Expr{&original}
					call.Fun = lit
					call.Args = nil
					call.Ellipsis = token.NoPos
					return false
				}
			}
			typ := resultType(call)
			if typ == "" {
				return true
			}
			var expression bytes.Buffer
			format.Node(&expression, sets, call.Fun)
			key := expression.String()
			counts[key]++
			id := fmt.Sprintf("%s/%s/%s/%d", filepath.Base(relative), fn.Name.Name, key, counts[key])
			points = append(points, point{id, fn.Name.Name})
			original := *call
			zero := "nil"
			if typ == "int" || typ == "int64" {
				zero = "0"
			}
			if typ == "string" {
				zero = `""`
			}
			signature := "error"
			returned := "_auditInjected"
			if typ == "bool" {
				signature = "bool"
				returned = "false"
			} else if typ != "error" {
				signature = "(" + typ + ",error)"
				returned = zero + ",_auditInjected"
			}
			source := fmt.Sprintf("func()%s{if _auditFail(%s){return %s};return PLACEHOLDER()}", signature, strconv.Quote(id), returned)
			parsed, parseErr := parser.ParseExpr(source)
			if parseErr != nil {
				panic(parseErr)
			}
			lit := parsed.(*ast.FuncLit)
			if selector, ok := original.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Close" {
				branch := lit.Body.List[0].(*ast.IfStmt)
				closed := original
				branch.Body.List = append([]ast.Stmt{&ast.ExprStmt{X: &closed}}, branch.Body.List...)
			}
			last := lit.Body.List[len(lit.Body.List)-1].(*ast.ReturnStmt)
			last.Results = []ast.Expr{&original}
			call.Fun = lit
			call.Args = nil
			call.Ellipsis = token.NoPos
			return false
		})
	}
	if len(points) == before {
		return nil
	}
	var output bytes.Buffer
	if err = format.Node(&output, sets, file); err != nil {
		return err
	}
	return os.WriteFile(path, output.Bytes(), 0600)
}

const helper = `package %s
import "example.com/go-block-lattice/internal/faultprobe"
var _auditInjected=faultprobe.Injected
func _auditArm(site string,nth int){faultprobe.Arm(site,nth)}
func _auditFail(site string)bool{return faultprobe.Fail(site)}
func _auditReached()bool{return faultprobe.Reached()}
var _auditPoints=[]struct{ID,Function string}{%s}
`
const probe = `package faultprobe
import("errors";"sync")
var Injected=errors.New("injected dependency failure")
var mu sync.Mutex
var selected string
var nth,count int
var action func()
func Arm(site string,n int){mu.Lock();defer mu.Unlock();selected=site;nth=n;count=0;action=nil}
func Before(site string,n int,fn func()){mu.Lock();defer mu.Unlock();selected=site;nth=n;count=0;action=fn}
func Fail(site string)bool{mu.Lock();if site!=selected{mu.Unlock();return false};count++;hit:=count==nth;fn:=action;mu.Unlock();if hit&&fn!=nil{fn();return false};return hit}
func Reached()bool{mu.Lock();defer mu.Unlock();return count>=nth && nth>0}
`

func main() {
	if len(os.Args) != 2 {
		panic("usage: instrument DISPOSABLE_COPY")
	}
	root, err := filepath.EvalSymlinks(os.Args[1])
	if err != nil {
		panic(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		panic(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		panic(err)
	}
	if root == cwd {
		panic("refusing working source tree")
	}
	if _, err := os.Lstat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		panic("refusing Git checkout")
	}
	dirs, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Name() == "branchprobe" {
			continue
		}
		points = nil
		files, err := filepath.Glob(filepath.Join(root, "internal", dir.Name(), "*.go"))
		if err != nil {
			panic(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			if err = instrument(path, filepath.Base(path)); err != nil {
				panic(err)
			}
		}
		if len(points) == 0 {
			continue
		}
		var list strings.Builder
		for _, p := range points {
			fmt.Fprintf(&list, "{%q,%q},", p.ID, p.Function)
		}
		content := fmt.Sprintf(helper, dir.Name(), list.String())
		if err = os.WriteFile(filepath.Join(root, "internal", dir.Name(), "audit_faults.go"), []byte(content), 0600); err != nil {
			panic(err)
		}
	}
	probeDir := filepath.Join(root, "internal", "faultprobe")
	if err = os.MkdirAll(probeDir, 0700); err != nil {
		panic(err)
	}
	if err = os.WriteFile(filepath.Join(probeDir, "probe.go"), []byte(probe), 0600); err != nil {
		panic(err)
	}

}
