package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"

	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

// Symbol kinds stored in the index.
const (
	KindFunc      = "func"
	KindMethod    = "method"
	KindType      = "type"
	KindStruct    = "struct"
	KindInterface = "interface"
	KindConst     = "const"
	KindVar       = "var"
)

// Reference edge kinds stored in index_refs.
const (
	// RefCall is a direct call of a function or method symbol.
	RefCall = "call"
	// RefUse is any other reference (value or type) to a package symbol.
	RefUse = "use"
)

// goFile is one parsed source file together with its file-scope symbols and
// imports. References are computed only after the whole package symbol table
// is known, so parsing stops here; reference extraction lives in refs.go.
type goFile struct {
	fset        *token.FileSet
	ast         *ast.File
	rel         string
	lang        string
	hash        string
	pkg         string
	symbols     []store.IndexSymbol
	imports     []string
	importNames map[string]bool
}

// parseGoPackageFile parses the Go source at f.abs.
func parseGoPackageFile(f indexedFile) (*goFile, error) {
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, f.abs, nil, 0)
	if err != nil {
		return nil, err
	}
	gf := &goFile{
		fset:        fset,
		ast:         af,
		rel:         f.rel,
		lang:        f.lang,
		hash:        f.hash,
		pkg:         af.Name.Name,
		symbols:     []store.IndexSymbol{},
		importNames: map[string]bool{},
	}
	gf.collectFileLevel(fset, af)
	return gf, nil
}

// collectFileLevel records file-scope symbols, imports, and the decl-node
// membership set used during reference resolution.
func (gf *goFile) collectFileLevel(fset *token.FileSet, af *ast.File) {
	for _, imp := range af.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		gf.imports = append(gf.imports, p)
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			gf.importNames[name] = true
		}
	}
	for _, decl := range af.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				recv := receiverType(d.Recv)
				if recv == "" {
					continue
				}
				gf.symbols = append(gf.symbols, store.IndexSymbol{
					Name: recv + "." + d.Name.Name,
					Kind: KindMethod,
					Line: fset.Position(d.Pos()).Line,
				})
			} else {
				gf.symbols = append(gf.symbols, store.IndexSymbol{
					Name: d.Name.Name,
					Kind: KindFunc,
					Line: fset.Position(d.Pos()).Line,
				})
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.Name == "_" {
						continue
					}
					kind := KindType
					switch s.Type.(type) {
					case *ast.StructType:
						kind = KindStruct
					case *ast.InterfaceType:
						kind = KindInterface
					}
					gf.symbols = append(gf.symbols, store.IndexSymbol{
						Name: s.Name.Name,
						Kind: kind,
						Line: fset.Position(s.Pos()).Line,
					})
				case *ast.ValueSpec:
					kind := KindVar
					if d.Tok == token.CONST {
						kind = KindConst
					}
					for _, n := range s.Names {
						if n.Name == "_" {
							continue
						}
						gf.symbols = append(gf.symbols, store.IndexSymbol{
							Name: n.Name,
							Kind: kind,
							Line: fset.Position(n.Pos()).Line,
						})
					}
				}
			}
		}
	}
	sortImports(gf.imports)
}

// receiverType extracts the base receiver type name from a method receiver,
// stripping pointers, parentheses, and type parameters (Pair[T] -> Pair).
func receiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	for {
		switch tt := t.(type) {
		case *ast.StarExpr:
			t = tt.X
		case *ast.ParenExpr:
			t = tt.X
		case *ast.IndexExpr:
			t = tt.X
		case *ast.IndexListExpr:
			t = tt.X
		default:
			goto done
		}
	}
done:
	id, ok := t.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func sortImports(imps []string) {
	for i := 1; i < len(imps); i++ {
		for j := i; j > 0 && imps[j] < imps[j-1]; j-- {
			imps[j], imps[j-1] = imps[j-1], imps[j]
		}
	}
}

// packageTable maps package-level symbol names to their defining file and
// kind for the whole package directory (a Go package spans one directory).
type packageTable struct {
	// symbols maps name -> entries. Method symbols are keyed by their
	// qualified receiver type (e.g. "Store.Save").
	symbols map[string]map[string]bool // name -> file set
	kinds   map[string]string
}

func newPackageTable() *packageTable {
	return &packageTable{symbols: map[string]map[string]bool{}, kinds: map[string]string{}}
}

// add records a symbol of file f.
func (pt *packageTable) add(name, kind, file string) {
	if pt.symbols[name] == nil {
		pt.symbols[name] = map[string]bool{}
	}
	pt.symbols[name][file] = true
	pt.kinds[name] = kind
}

// kind returns the kind of a package-level symbol name.
func (pt *packageTable) kind(name string) (string, bool) {
	k, ok := pt.kinds[name]
	return k, ok
}

// resolve returns the file set declaring name in this package.
func (pt *packageTable) resolve(name string) (map[string]bool, string, bool) {
	files, ok := pt.symbols[name]
	if !ok {
		return nil, "", false
	}
	return files, pt.kinds[name], true
}

// firstFile returns the lexicographically smallest file declaring name; a
// duplicate declaration is a compile error, so any deterministic pick works.
func (pt *packageTable) firstFile(name string) string {
	files, _, ok := pt.resolve(name)
	if !ok {
		return ""
	}
	best := ""
	for f := range files {
		if best == "" || f < best {
			best = f
		}
	}
	return best
}
