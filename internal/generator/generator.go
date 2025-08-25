package generator

import (
	"crypto/sha1"
	"encoding/hex"
	"go/types"
	"strings"
)

// generator holds transient state while building models.
type generator struct {
	currentPkgName string
	methodMap      map[string]methodInfo // key: src->dest
	helperSet      map[string]bool
	helperModels   []helperModel
	helperErrors   map[string]bool // helper name -> returns error
	pkgScope       *types.Scope
	currentCtxName string                // set while building a method if ctx parameter present
	customFuncs    map[string]methodInfo // key: src->dest or src->dest#err for error variant
}

func Run(cfg Config) error { return newGenerator().run(cfg) }

func newGenerator() *generator {
	return &generator{
		methodMap:    make(map[string]methodInfo),
		helperSet:    make(map[string]bool),
		helperErrors: make(map[string]bool),
	}
}

func (g *generator) qualifier(p *types.Package) string {
	if p == nil || p.Name() == g.currentPkgName {
		return ""
	}
	return p.Name()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToLower(string(r[0])))[0]
	return string(r)
}

func isErrorType(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		if named.Obj().Pkg() == nil && named.Obj().Name() == "error" {
			return true
		}
	}
	return false
}

func underlyingStruct(t types.Type) (*types.Struct, bool) {
	switch tt := t.(type) {
	case *types.Pointer:
		if s, ok := tt.Elem().Underlying().(*types.Struct); ok {
			return s, true
		}
	default:
		if s, ok := tt.Underlying().(*types.Struct); ok {
			return s, false
		}
	}
	return nil, false
}

func (g *generator) zeroValue(t types.Type) string {
	if _, ok := t.(*types.Pointer); ok {
		return "nil"
	}
	return types.TypeString(t, g.qualifier) + "{}"
}

func isStructLike(t types.Type) bool {
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

func isCollectionLike(t types.Type) bool {
	switch t.(type) {
	case *types.Slice, *types.Array, *types.Map:
		return true
	}
	return false
}

func prefixDest(destPtr bool) string {
	if destPtr {
		return "mapped."
	}
	return "dst."
}

func prefixSrc(param string, isPtr bool) string {
	if isPtr {
		return "*" + param + "."
	}
	return param + "."
}

func helperNameFor(key string) string {
	h := sha1.Sum([]byte(key))
	return "map_" + hex.EncodeToString(h[:6])
}

func findMatchingSourceField(src *types.Struct, name string) *types.Var {
	for i := 0; i < src.NumFields(); i++ {
		f := src.Field(i)
		if f.Exported() && f.Name() == name {
			return f
		}
	}
	return nil
}

func findTaggedSourceField(src *types.Struct, destName string) *types.Var {
	for i := 0; i < src.NumFields(); i++ {
		f := src.Field(i)
		if !f.Exported() {
			continue
		}
		if tag := src.Tag(i); tag != "" {
			parsed := parseTag(tag)
			if v, ok := parsed["map"]; ok && strings.EqualFold(v, destName) {
				return f
			}
		}
	}
	return nil
}

func parseTag(tag string) map[string]string {
	res := map[string]string{}
	tag = strings.Trim(tag, "`")
	for _, p := range strings.Split(tag, " ") {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, ":") {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		k := kv[0]
		v := strings.Trim(kv[1], "\"")
		v = strings.Trim(v, "\"")
		res[k] = v
	}
	return res
}

func customFuncKey(src, dest string, hasErr bool) string {
	if hasErr {
		return src + "->" + dest + "#err"
	}
	return src + "->" + dest
}
