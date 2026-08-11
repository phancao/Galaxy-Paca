// authzgate — ADR-039 A2 ratchet for Paca's HTTP routes.
//
// It answers one question: does every state-changing route (POST/PUT/PATCH/
// DELETE) run a permission check before its handler?
//
// # WHY AN AST WALK AND NOT A GREP
//
// Paca's router is chi with closures:
//
//	r.Route("/admin", func(r chi.Router) {
//	    r.Use(httpmw.Authn(...))                      // authn, NOT authz
//	    r.With(httpmw.RequirePermissions(...)).Post(...)  // authz, inline
//	})
//
// Two traps a grep falls into, and both bit me while measuring this repo:
//
//  1. Authorization lives in `.With(...)` on the route, not only in `.Use(...)`
//     on the group. Counting `Use(` alone reported 2 permission checks where
//     there are 12, and made a well-guarded router look unguarded.
//  2. Every nesting level is named `r`, because each closure shadows the last.
//     Tracking middleware by receiver NAME — which the sibling gate in
//     Galaxy-Innovation-Platform does, correctly, for its router — merges all
//     levels into one and credits a child's middleware to its parent.
//
// So inheritance here is LEXICAL: a route inherits the Use() calls of the
// closure bodies it sits inside, and nothing else.
//
// AUTHENTICATION IS NOT AUTHORIZATION. `Authn`, `OptionalAuthn`,
// `RequireJWTAuth` and `RequireFreshPassword` all establish *who* is calling.
// None answers *may they do this*. Only the first list below counts.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Middleware that decides whether THIS caller may perform THIS action.
var authzMiddleware = map[string]bool{
	"RequirePermissions":    true,
	"RequireGlobalRole":     true,
	"RequireProjectRole":    true,
	"adminClaimsMiddleware": true,
}

// Middleware that only establishes identity. Listed explicitly so that adding
// one here can never be mistaken for adding a guard.
var authnOnly = map[string]bool{
	"Authn":                      true,
	"OptionalAuthn":              true,
	"RequireJWTAuth":             true,
	"RequireFreshPassword":       true,
	"claimsMiddleware":           true,
	"injectAuthClaimsMiddleware": true,
	"requestIDMiddleware":        true,
	"loggerMiddleware":           true,
	"corsMiddleware":             true,
	"Recoverer":                  true,
}

var stateMethods = map[string]bool{
	"Post": true, "Put": true, "Patch": true, "Delete": true,
}

type finding struct {
	file   string
	line   int
	method string
	path   string
}

func (f finding) key() string {
	return fmt.Sprintf("%s::%s %s", f.file, f.method, f.path)
}

// middlewareNames returns every function name mentioned in a middleware
// argument, so `httpmw.RequirePermissions(deps.Authorizer, ...)` yields
// "RequirePermissions".
func middlewareNames(e ast.Expr) []string {
	var out []string
	ast.Inspect(e, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			out = append(out, v.Sel.Name)
		case *ast.Ident:
			out = append(out, v.Name)
		}
		return true
	})
	return out
}

func hasAuthz(e ast.Expr) bool {
	for _, n := range middlewareNames(e) {
		if authzMiddleware[n] {
			return true
		}
	}
	return false
}

// walk descends the router closures. `inherited` is true when some enclosing
// closure already applied an authorization middleware via Use().
func walk(fset *token.FileSet, file string, body *ast.BlockStmt, inherited bool, out *[]finding) {
	// What THIS body's own Use() calls contribute. chi applies a Use() to every
	// route registered on that router regardless of where the Use() sits, so
	// unlike the Innovation router this is not positional — but it IS scoped to
	// one level.
	//
	// Walked with an explicit loop over body.List rather than ast.Inspect: an
	// Inspect descends into the nested closures too, so a Use() deep in a child
	// marked every ancestor as guarded and the gate reported zero findings on a
	// router with nine unguarded routes. A gate that finds nothing is the failure
	// this tool exists to prevent, so it must not contain it.
	guarded := inherited
	for _, call := range callsAtLevel(body) {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Use" {
			continue
		}
		for _, arg := range call.Args {
			if hasAuthz(arg) {
				guarded = true
			}
		}
	}

	// Now the routes and the nested closures, at THIS level only.
	for _, call := range callsAtLevel(body) {
		inspectCall(fset, file, call, guarded, out)
	}
}

// callsAtLevel returns the calls that belong to THIS router, seeing through
// control flow but not through a nested closure.
//
// Routes are frequently registered conditionally:
//
//	if deps.APIKey != nil {
//	    r.Post("/me/api-keys", ...)
//	}
//
// Reading only the direct ExprStmts of the body missed four such routes here,
// which would have let any conditionally-registered route escape the gate
// entirely. Nested FuncLits are deliberately NOT descended into: those are new
// routers and walk() handles them with their own inherited state.
func callsAtLevel(body *ast.BlockStmt) []*ast.CallExpr {
	var out []*ast.CallExpr
	var visit func(stmts []ast.Stmt)
	visit = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch v := stmt.(type) {
			case *ast.ExprStmt:
				if call, ok := v.X.(*ast.CallExpr); ok {
					out = append(out, call)
				}
			case *ast.IfStmt:
				if v.Body != nil {
					visit(v.Body.List)
				}
				switch e := v.Else.(type) {
				case *ast.BlockStmt:
					visit(e.List)
				case *ast.IfStmt:
					visit([]ast.Stmt{e})
				}
			case *ast.BlockStmt:
				visit(v.List)
			case *ast.ForStmt:
				if v.Body != nil {
					visit(v.Body.List)
				}
			case *ast.RangeStmt:
				if v.Body != nil {
					visit(v.Body.List)
				}
			case *ast.SwitchStmt:
				if v.Body != nil {
					visit(v.Body.List)
				}
			case *ast.TypeSwitchStmt:
				if v.Body != nil {
					visit(v.Body.List)
				}
			case *ast.CaseClause:
				visit(v.Body)
			}
		}
	}
	visit(body.List)
	return out
}

func inspectCall(fset *token.FileSet, file string, call *ast.CallExpr, guarded bool, out *[]finding) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	name := sel.Sel.Name

	// r.Route("/x", func(r chi.Router){...})  /  r.Group(func(r chi.Router){...})
	if name == "Route" || name == "Group" {
		for _, arg := range call.Args {
			if fn, ok := arg.(*ast.FuncLit); ok {
				walk(fset, file, fn.Body, guarded, out)
			}
		}
		return
	}

	// r.With(mw...).Post("/x", h) — the With() is the receiver of the route call.
	if stateMethods[name] {
		inline := false
		if withCall, ok := sel.X.(*ast.CallExpr); ok {
			if withSel, ok := withCall.Fun.(*ast.SelectorExpr); ok && withSel.Sel.Name == "With" {
				for _, arg := range withCall.Args {
					if hasAuthz(arg) {
						inline = true
					}
				}
			}
		}
		if guarded || inline {
			return
		}
		path := "?"
		if len(call.Args) > 0 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				path = strings.Trim(lit.Value, `"`)
			}
		}
		*out = append(*out, finding{
			file:   file,
			line:   fset.Position(call.Pos()).Line,
			method: strings.ToUpper(name),
			path:   path,
		})
		return
	}

	// Anything else may still wrap a closure (e.g. r.With(...).Route(...)).
	for _, arg := range call.Args {
		if fn, ok := arg.(*ast.FuncLit); ok {
			walk(fset, file, fn.Body, guarded, out)
		}
	}
}

func scan(path string) []finding {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "authzgate: cannot parse %s: %v\n", path, err)
		return nil
	}
	var out []finding
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		walk(fset, path, fn.Body, false, &out)
		return false
	})
	return out
}

func loadBaseline(path string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	baselinePath := filepath.Join("tools", "authzgate", "baseline.txt")
	baseline := loadBaseline(baselinePath)

	var all []finding
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(p)
			if base == "vendor" || base == "node_modules" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		all = append(all, scan(p)...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "authzgate:", err)
		os.Exit(2)
	}

	var netNew []finding
	for _, f := range all {
		if !baseline[f.key()] {
			netNew = append(netNew, f)
		}
	}
	sort.Slice(netNew, func(i, j int) bool { return netNew[i].key() < netNew[j].key() })

	if len(os.Args) > 2 && os.Args[2] == "--write-baseline" {
		var keys []string
		for _, f := range all {
			keys = append(keys, f.key())
		}
		sort.Strings(keys)
		fmt.Println(strings.Join(keys, "\n"))
		return
	}

	if len(netNew) > 0 {
		fmt.Println("❌ ADR-039 A2: state-changing route with no permission check (net-new, outside the baseline):")
		for _, f := range netNew {
			fmt.Printf("   %s:%d  %s %s\n", f.file, f.line, f.method, f.path)
		}
		fmt.Println()
		fmt.Println("Fix: put the route behind httpmw.RequirePermissions(deps.Authorizer, <scope>, <permission>),")
		fmt.Println("either inline with .With(...) or via Use() on its group.")
		fmt.Println("Authn / RequireJWTAuth / RequireFreshPassword do NOT count: they establish who is")
		fmt.Println("calling, not whether they may do this.")
		fmt.Println("If the route is deliberately self-scoped (/me/...) or part of login, add it to")
		fmt.Println("tools/authzgate/baseline.txt with a reason.")
		os.Exit(1)
	}

	fmt.Printf("✅ authzgate OK — %d state-changing route(s) without a permission check, all baselined.\n", len(all))
}
