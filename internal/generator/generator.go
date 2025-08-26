package generator

import (
	"fmt"
	"go/types"
	"regexp"
	"strings"
	"unicode/utf8"
)

// generator holds transient state while building models.
type generator struct {
	currentPkgName string
	// registry maps src->dest (and src->dest#err) to metadata for interface
	// methods or custom funcs.
	registry     map[string]registryEntry
	helperNames  map[string]string // key -> helper function name
	helperModels []helperModel
	helperPlans  []helperPlan // planning data for two-pass population
	resolver     *fieldResolver
}

// helperPlan stores planning metadata prior to IR helperModel population.
type helperPlan struct {
	name               string
	srcType            types.Type
	destType           types.Type
	srcIsPtr           bool
	destIsPtr          bool
	underDestType      string
	zeroReturn         string
	customFuncName     string
	customFuncHasError bool
	populated          bool
	composite          bool // true for top-level collection/map helpers
}

// methodPlan stores method signature and high-level mapping classification
// prior to node construction.
type methodPlan struct {
	name             string
	signature        *types.Signature
	params           []paramModel // ordered (excluding synthesized names?)
	primaryIndex     int
	ctxIndex         int
	hasError         bool
	structMapping    bool
	compositeMapping bool
	implName         string
}

// Run executes the generation with the provided configuration.
// Accepts a pointer to avoid copying a large struct (lint hugeParam).
func Run(cfg *Config) error { return newGenerator().run(*cfg) }

func newGenerator() *generator {
	g := &generator{
		registry:    make(map[string]registryEntry),
		helperNames: make(map[string]string),
	}

	g.resolver = &fieldResolver{g: g}
	return g
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
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || size == 0 {
		return s
	}
	lower := strings.ToLower(string(r))
	if lower == "" {
		return s
	}
	return lower + s[size:]
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

// helperName derives a deterministic (readable) name. Format:
// map_<Src>_to_<Dest>_<N> where Src/Dest are simplified type tokens.
func (g *generator) helperName(srcType, destType types.Type, composite bool) string {
	// Encode structure of types so names are stable and collision-free across runs.
	var tok func(types.Type) string
	tok = func(t types.Type) string {
		switch tt := t.(type) {
		case *types.Pointer:
			return fmt.Sprintf("Ptr_%s", tok(tt.Elem()))
		case *types.Slice:
			return fmt.Sprintf("Slice_%s", tok(tt.Elem()))
		case *types.Array:
			return fmt.Sprintf("Array_%d_%s", tt.Len(), tok(tt.Elem()))
		case *types.Map:
			return "Map_" + tok(tt.Key()) + "_To_" + tok(tt.Elem())
		case *types.Named:
			obj := tt.Obj()
			if obj != nil {
				if obj.Pkg() != nil && obj.Pkg().Name() != g.currentPkgName {
					return fmt.Sprintf("%s_%s", obj.Pkg().Name(), obj.Name())
				}
				return obj.Name()
			}
		}
		// Fallback: sanitize the type string.
		s := types.TypeString(t, g.qualifier)
		re := regexp.MustCompile(`[^A-Za-z0-9]+`)
		s = re.ReplaceAllString(s, "_")
		s = strings.Trim(s, "_")
		if s == "" {
			return "T"
		}
		return s
	}
	prefix := "map"
	if composite {
		prefix = "mapc"
	}
	return prefix + "_" + tok(srcType) + "_to_" + tok(destType)
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

// package-level tag cache (single-threaded generator run)
var tagCache = map[*types.Struct]map[int]map[string]string{}

func parseTagCached(s *types.Struct, i int) map[string]string {
	if s == nil || i < 0 || i >= s.NumFields() {
		return nil
	}
	fm := tagCache[s]
	if fm == nil {
		fm = make(map[int]map[string]string)
		tagCache[s] = fm
	}
	if cached, ok := fm[i]; ok {
		return cached
	}
	raw := s.Tag(i)
	if raw == "" {
		fm[i] = nil
		return nil
	}
	parsed := parseTag(raw)
	fm[i] = parsed
	return parsed
}

func customFuncKey(src, dest string, hasErr bool) string {
	if hasErr {
		return src + "->" + dest + "#err"
	}
	return src + "->" + dest
}

// findCustomVariant returns a custom func variant (non-error or error) if
// present for the base key.
func (g *generator) findCustomVariant(base string) *registryEntry {
	if e, ok := g.registry[base]; ok && e.Kind == regKindCustomFunc {
		return &e
	}
	if e, ok := g.registry[base+"#err"]; ok && e.Kind == regKindCustomFunc {
		return &e
	}
	return nil
}
