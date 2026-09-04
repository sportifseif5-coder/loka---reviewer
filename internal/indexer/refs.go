package indexer

import (
	"go/ast"
	"go/token"
	"sort"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// indexGoDir parses every candidate file of a package directory and returns
// its deterministic directory index. Files are grouped by package clause so
// that an external _test package resolves references only against its own
// symbol table. Parse failures are returned in skipped and excluded from the
// index: a single broken file never drops the rest of a package.
func indexGoDir(dir string, files []indexedFile) (*store.DirIndex, []string) {
	di := &store.DirIndex{Dir: dir}
	var skipped []string
	byRel := map[string]*goFile{}
	groups := map[string][]string{} // package name -> sorted file rels
	for _, f := range files {
		if f.lang != "go" {
			continue
		}
		gf, err := parseGoPackageFile(f)
		if err != nil {
			skipped = append(skipped, f.rel)
			continue
		}
		byRel[gf.rel] = gf
		groups[gf.pkg] = append(groups[gf.pkg], gf.rel)
	}

	di.Files = make([]store.IndexFile, 0, len(byRel))
	rels := make([]string, 0, len(byRel))
	for rel := range byRel {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		di.Files = append(di.Files, store.IndexFile{
			Path:     rel,
			Language: "go",
			Hash:     byRel[rel].hash,
			Symbols:  sortedSymbols(byRel[rel].symbols),
			Imports:  append([]string(nil), byRel[rel].imports...),
		})
	}

	pkgs := make([]string, 0, len(groups))
	for p := range groups {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	refSeen := map[string]bool{}
	for _, pkg := range pkgs {
		pt := newPackageTable()
		fileRels := groups[pkg]
		sort.Strings(fileRels)
		for _, rel := range fileRels {
			for _, s := range byRel[rel].symbols {
				pt.add(s.Name, s.Kind, rel)
			}
		}
		for _, rel := range fileRels {
			c := newRefCollector(byRel[rel], pt)
			for _, r := range c.out {
				key := r.SrcFile + "\x00" + r.SrcName + "\x00" + r.DstFile +
					"\x00" + r.DstName + "\x00" + r.Kind
				if refSeen[key] {
					continue
				}
				refSeen[key] = true
				di.Refs = append(di.Refs, r)
			}
		}
	}
	sort.Slice(di.Refs, func(i, j int) bool {
		a, b := di.Refs[i], di.Refs[j]
		if a.SrcFile != b.SrcFile {
			return a.SrcFile < b.SrcFile
		}
		if a.SrcName != b.SrcName {
			return a.SrcName < b.SrcName
		}
		if a.DstFile != b.DstFile {
			return a.DstFile < b.DstFile
		}
		if a.DstName != b.DstName {
			return a.DstName < b.DstName
		}
		return a.Kind < b.Kind
	})
	return di, skipped
}

// sortedSymbols orders symbols by line then name, matching the read path.
func sortedSymbols(syms []store.IndexSymbol) []store.IndexSymbol {
	out := append([]store.IndexSymbol(nil), syms...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// refCollector walks one file and records every intra-package reference. It
// is a structural resolver, not a type checker: it emits an edge when an
// identifier in a reference position names a symbol in the package table and
// is not shadowed by a lexical binding. This is deterministic and offline;
// it can miss (never fabricate) edges that need go/types.
type refCollector struct {
	gf     *goFile
	pt     *packageTable
	parent map[ast.Node]ast.Node
	// scopes holds the lexically active binding scopes; the innermost is
	// last. Parameter, receiver, short-variable, and range names live here
	// so they shadow package symbols of the same name.
	scopes []map[string]bool
	// syms holds the stack of enclosing file-scope symbol names; a
	// reference is attributed to the innermost symbol that owns it.
	syms []string
	out  []store.IndexRef
	seen map[string]bool
}

func newRefCollector(gf *goFile, pt *packageTable) *refCollector {
	c := &refCollector{gf: gf, pt: pt, seen: map[string]bool{}}
	c.buildParent()
	c.walk()
	c.sortOut()
	return c
}

// buildParent records the parent of every AST node in the file.
func (c *refCollector) buildParent() {
	c.parent = map[ast.Node]ast.Node{}
	stack := []ast.Node{c.gf.ast}
	ast.Inspect(c.gf.ast, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if len(stack) > 0 && stack[len(stack)-1] != n {
			c.parent[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
}

// walk visits the file in document order, managing scope and symbol stacks
// and emitting references for identifiers in reference positions.
func (c *refCollector) walk() {
	open := []ast.Node{}
	ast.Inspect(c.gf.ast, func(n ast.Node) bool {
		if n == nil {
			if len(open) == 0 {
				return true
			}
			top := open[len(open)-1]
			open = open[:len(open)-1]
			c.leave(top)
			return true
		}
		open = append(open, n)
		c.enter(n)
		if id, ok := n.(*ast.Ident); ok {
			c.use(id)
		}
		return true
	})
}

// enter runs before a node's subtree is visited.
func (c *refCollector) enter(n ast.Node) {
	switch t := n.(type) {
	case *ast.FuncDecl:
		c.pushScope()
		c.addLocal(fieldNames(t.Recv)...)
		if ft := t.Type; ft != nil {
			c.addLocal(fieldNames(ft.TypeParams)...)
			c.addLocal(fieldNames(ft.Params)...)
			c.addLocal(fieldNames(ft.Results)...)
		}
		c.pushSym(funcSymbolName(t))
	case *ast.FuncLit:
		c.pushScope()
		if ft := t.Type; ft != nil {
			c.addLocal(fieldNames(ft.Params)...)
			c.addLocal(fieldNames(ft.Results)...)
		}
	case *ast.TypeSpec:
		c.pushScope()
		c.addLocal(fieldNames(t.TypeParams)...)
		if c.fileScope(t) && t.Name.Name != "_" {
			c.pushSym(t.Name.Name)
		}
	case *ast.ValueSpec:
		if c.fileScope(t) {
			for _, n := range t.Names {
				if n.Name != "_" {
					c.pushSym(n.Name)
					break
				}
			}
		}
	case *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
		*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
		*ast.CaseClause, *ast.CommClause:
		c.pushScope()
	}
}

// leave runs after a node's subtree is fully visited.
func (c *refCollector) leave(n ast.Node) {
	switch t := n.(type) {
	case *ast.FuncDecl:
		c.popScope()
		c.popSym()
	case *ast.TypeSpec:
		c.popScope()
		if c.fileScope(t) && t.Name.Name != "_" {
			c.popSym()
		}
	case *ast.FuncLit, *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt,
		*ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt,
		*ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
		c.popScope()
	case *ast.ValueSpec:
		if c.fileScope(t) {
			for _, n := range t.Names {
				if n.Name != "_" {
					c.popSym()
					break
				}
			}
		}
	}
}

// fileScope reports whether a spec node is a direct child of the file
// declaration block rather than nested inside a function.
func (c *refCollector) fileScope(n ast.Node) bool {
	parent, ok := c.parent[n].(*ast.GenDecl)
	if !ok {
		return false
	}
	_, isFile := c.parent[parent].(*ast.File)
	return isFile
}

func (c *refCollector) pushScope() { c.scopes = append(c.scopes, map[string]bool{}) }
func (c *refCollector) popScope()  { c.scopes = c.scopes[:len(c.scopes)-1] }
func (c *refCollector) pushSym(s string) {
	c.syms = append(c.syms, s)
}
func (c *refCollector) popSym() { c.syms = c.syms[:len(c.syms)-1] }

// addLocal binds names into the innermost scope.
func (c *refCollector) addLocal(names ...string) {
	if len(c.scopes) == 0 {
		return
	}
	top := c.scopes[len(c.scopes)-1]
	for _, n := range names {
		if n != "_" {
			top[n] = true
		}
	}
}

// isLocal reports whether name is bound by any open scope (innermost first).
func (c *refCollector) isLocal(name string) bool {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if c.scopes[i][name] {
			return true
		}
	}
	return false
}

func (c *refCollector) isImport(name string) bool { return c.gf.importNames[name] }

// use decides whether one identifier is a reference to a package symbol and,
// when it is, records the edge. Binding names (declarations, parameters,
// fields, short variables) never become references.
func (c *refCollector) use(id *ast.Ident) {
	name := id.Name
	if name == "_" {
		return
	}
	p, ok := c.parent[id]
	if !ok || p == nil {
		return
	}
	switch t := p.(type) {
	case *ast.ValueSpec:
		for _, n := range t.Names {
			if n == id {
				return
			}
		}
	case *ast.TypeSpec:
		if t.Name == id {
			return
		}
	case *ast.FuncDecl:
		if t.Name == id {
			return
		}
	case *ast.Field:
		for _, n := range t.Names {
			if n == id {
				return
			}
		}
	case *ast.LabeledStmt:
		if t.Label == id {
			return
		}
	case *ast.BranchStmt:
		if t.Label == id {
			return
		}
	case *ast.ImportSpec:
		if t.Name == id {
			return
		}
	case *ast.KeyValueExpr:
		if t.Key == id && c.isStructKey(t) {
			return
		}
	case *ast.AssignStmt:
		if t.Tok == token.DEFINE && c.isDeclaredLhs(t.Lhs, id) {
			c.addLocal(name)
			return
		}
	case *ast.RangeStmt:
		if t.Tok == token.DEFINE && (c.isBoundIdent(t.Key, id) || c.isBoundIdent(t.Value, id)) {
			c.addLocal(name)
			return
		}
	case *ast.SelectorExpr:
		if t.Sel == id {
			return // member names are never package symbols
		}
		if c.isLocal(name) || c.isImport(name) {
			return
		}
		c.selectorBase(t, id)
		return
	case *ast.CallExpr:
		if t.Fun == id {
			if c.isLocal(name) {
				return
			}
			if kind, ok := c.pt.kind(name); ok {
				if kind == KindFunc {
					c.emit(name, RefCall)
				} else {
					c.emit(name, RefUse)
				}
			}
			return
		}
	}

	// A plain value/type reference.
	if c.isLocal(name) || c.isImport(name) {
		return
	}
	if _, ok := c.pt.kind(name); ok {
		c.emit(name, RefUse)
	}
}

func (c *refCollector) isDeclaredLhs(lhs []ast.Expr, id *ast.Ident) bool {
	for _, e := range lhs {
		if n, ok := e.(*ast.Ident); ok && n == id {
			return true
		}
	}
	return false
}

func (c *refCollector) isBoundIdent(e ast.Expr, id *ast.Ident) bool {
	n, ok := e.(*ast.Ident)
	return ok && n == id
}

// isStructKey reports whether an identifier key belongs to a struct literal;
// map literal keys are expressions and may reference symbols.
func (c *refCollector) isStructKey(kv *ast.KeyValueExpr) bool {
	parent, ok := c.parent[kv].(*ast.CompositeLit)
	if !ok {
		return true
	}
	_, isMap := parent.Type.(*ast.MapType)
	return !isMap
}

// selectorBase resolves the base of a selector such as Store.Save or
// db.Query. Imported qualifiers and locals were already excluded by use.
func (c *refCollector) selectorBase(sel *ast.SelectorExpr, base *ast.Ident) {
	if _, ok := c.pt.kind(base.Name); !ok {
		return
	}
	method := base.Name + "." + sel.Sel.Name
	if _, isMethod := c.pt.kind(method); isMethod {
		if call, isCall := c.parent[sel].(*ast.CallExpr); isCall && call.Fun == sel {
			c.emit(method, RefCall)
		} else {
			c.emit(method, RefUse)
		}
		return
	}
	// No method of that name on this receiver: the base itself is the
	// referenced symbol (a type value, variable, or constant).
	c.emit(base.Name, RefUse)
}

// emit records one reference edge, attributed to the innermost enclosing
// file-scope symbol, and drops self-edges and duplicates.
func (c *refCollector) emit(name, kind string) {
	if len(c.syms) == 0 {
		return
	}
	src := c.syms[len(c.syms)-1]
	if src == "" {
		return
	}
	dstFile := c.pt.firstFile(name)
	if dstFile == "" {
		return
	}
	if dstFile == c.gf.rel && src == name {
		return
	}
	key := c.gf.rel + "\x00" + src + "\x00" + dstFile + "\x00" + name + "\x00" + kind
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.out = append(c.out, store.IndexRef{
		SrcFile: c.gf.rel,
		SrcName: src,
		DstFile: dstFile,
		DstName: name,
		Kind:    kind,
	})
}

func (c *refCollector) sortOut() {
	sort.Slice(c.out, func(i, j int) bool {
		a, b := c.out[i], c.out[j]
		if a.SrcFile != b.SrcFile {
			return a.SrcFile < b.SrcFile
		}
		if a.SrcName != b.SrcName {
			return a.SrcName < b.SrcName
		}
		if a.DstFile != b.DstFile {
			return a.DstFile < b.DstFile
		}
		if a.DstName != b.DstName {
			return a.DstName < b.DstName
		}
		return a.Kind < b.Kind
	})
}

// fieldNames returns every bound name declared by a field list.
func fieldNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var out []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// funcSymbolName names a function or method symbol as stored by
// collectFileLevel (methods are qualified by their receiver base type).
func funcSymbolName(fd *ast.FuncDecl) string {
	if fd.Recv == nil {
		return fd.Name.Name
	}
	recv := receiverType(fd.Recv)
	if recv == "" {
		return ""
	}
	return recv + "." + fd.Name.Name
}
