package generator

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadDir loads the Go package(s) for a directory.
func loadDir(dir string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, "./")
	if err != nil {
		return nil, err
	}
	var result []*packages.Package
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, p.Errors[0]
		}
		result = append(result, p)
	}
	return result, nil
}

// buildInterfaceModel constructs the model for a single interface type.
func (g *generator) buildInterfaceModel(name string, iface *types.Interface) (*interfaceModel, error) {
	implName := lowerFirst(name) + "Impl"
	im := &interfaceModel{Name: name, ImplName: implName}
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		if sig, ok := m.Type().(*types.Signature); ok && sig.Params().Len() >= 1 && sig.Results().Len() >= 1 {
			primaryIdx := 0
			srcT := types.TypeString(sig.Params().At(primaryIdx).Type(), g.qualifier)
			destT := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
			key := srcT + "->" + destT
			// skip adding interface method mapping if ANY custom variant exists
			_, nonErr := g.customFuncs[key]
			_, errVar := g.customFuncs[key+"#err"]
			if !nonErr && !errVar {
				g.methodMap[key] = methodInfo{Name: m.Name(), HasError: sig.Results().Len() == 2, IsFunc: false}
			}
		}
		mm, err := g.buildMethodModel(implName, m)
		if err != nil {
			return nil, fmt.Errorf("method %s: %w", m.Name(), err)
		}
		im.Methods = append(im.Methods, *mm)
	}
	return im, nil
}

// discoverCustomFuncs finds eligible custom mapping functions.
func (g *generator) discoverCustomFuncs(pkg *packages.Package, allowlist []string) map[string]methodInfo {
	allowed := map[string]bool{}
	if len(allowlist) > 0 {
		for _, n := range allowlist {
			allowed[n] = true
		}
	}
	res := map[string]methodInfo{}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		if len(allowlist) > 0 && !allowed[name] {
			continue
		}
		obj := scope.Lookup(name)
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 1 || sig.Results().Len() < 1 || sig.Results().Len() > 2 {
			continue
		}
		if sig.Results().Len() == 2 && !isErrorType(sig.Results().At(1).Type()) {
			continue
		}
		srcT := types.TypeString(sig.Params().At(0).Type(), g.qualifier)
		destT := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
		key := customFuncKey(srcT, destT, sig.Results().Len() == 2)
		if _, exists := g.methodMap[srcT+"->"+destT]; exists {
			continue
		}
		res[key] = methodInfo{Name: name, HasError: sig.Results().Len() == 2, IsFunc: true}
	}
	return res
}

// Orchestrates initial discovery and interface model construction.
func (g *generator) discoverAndBuild(cfg Config, pkg *packages.Package, ifaceMap map[string]*types.Interface) ([]interfaceModel, error) {
	// keep stable ordering
	sort.Strings(cfg.Interfaces)
	// discover custom funcs
	funcMap := g.discoverCustomFuncs(pkg, cfg.CustomFuncs)
	if g.customFuncs == nil {
		g.customFuncs = make(map[string]methodInfo)
	}
	for k, mi := range funcMap {
		g.customFuncs[k] = mi
		if !mi.HasError { // register non-error variant for nested use
			base := strings.TrimSuffix(k, "#err")
			if _, ok := g.methodMap[base]; !ok {
				g.methodMap[base] = mi
			}
		}
	}
	g.pkgScope = pkg.Types.Scope()

	var interfaceModels []interfaceModel
	for _, name := range cfg.Interfaces {
		model, err := g.buildInterfaceModel(name, ifaceMap[name])
		if err != nil {
			return nil, err
		}
		interfaceModels = append(interfaceModels, *model)
	}
	return interfaceModels, nil
}
