package generator

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// run orchestrates discovery, modeling, analysis, and file emission.
func (g *generator) run(cfg Config) error {
	if len(cfg.Interfaces) == 0 {
		return errors.New("no interfaces provided")
	}
	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return err
	}
	pkgs, err := loadDir(absDir)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found in %s", absDir)
	}
	pkg := pkgs[0]
	g.currentPkgName = pkg.Name

	ifaceMap := map[string]*types.Interface{}
	missing := []string{}
	scope := pkg.Types.Scope()
	for _, name := range cfg.Interfaces {
		obj := scope.Lookup(name)
		if obj == nil {
			missing = append(missing, name)
			continue
		}
		t, ok := obj.Type().Underlying().(*types.Interface)
		if !ok {
			return fmt.Errorf("%s is not an interface", name)
		}
		ifaceMap[name] = t
	}
	if len(missing) > 0 {
		return fmt.Errorf("interfaces not found: %s", strings.Join(missing, ", "))
	}
	sort.Strings(cfg.Interfaces)
	g.helperSet = make(map[string]bool)
	g.helperModels = nil
	funcMap := g.discoverCustomFuncs(pkg, cfg.CustomFuncs)
	if g.customFuncs == nil {
		g.customFuncs = make(map[string]methodInfo)
	}
	for k, mi := range funcMap {
		g.customFuncs[k] = mi
		if !mi.HasError {
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
			return err
		}
		interfaceModels = append(interfaceModels, *model)
	}

	g.computeHelperErrors()
	g.annotateHelperErrorUsage(&interfaceModels)
	needCtx := false
	for _, im := range interfaceModels {
		for _, mm := range im.Methods {
			if mm.HasContext {
				needCtx = true
				break
			}
		}
	}
	if needCtx {
		for i := range g.helperModels {
			g.helperModels[i].HasContext = true
		}
	}
	if cfg.Debug {
		var assignPaths func(prefix string, nodes []codeNode)
		assignPaths = func(prefix string, nodes []codeNode) {
			for i := range nodes {
				nodes[i].Debug = true
				nodes[i].Path = prefix + fmt.Sprintf(".%d", i)
				if len(nodes[i].Children) > 0 {
					assignPaths(nodes[i].Path, nodes[i].Children)
				}
			}
		}
		for hi := range g.helperModels {
			assignPaths(fmt.Sprintf("H%d", hi), g.helperModels[hi].Body)
		}
		for ii := range interfaceModels {
			im := &interfaceModels[ii]
			for mi := range im.Methods {
				assignPaths(fmt.Sprintf("I%d.M%d", ii, mi), im.Methods[mi].Body)
			}
		}
	}
	data := fileModel{Package: pkg.Name, Source: strings.Join(cfg.Interfaces, ", "), Helpers: g.helperModels, Interfaces: interfaceModels, NeedContext: needCtx, Debug: cfg.Debug, Command: cfg.Command, Version: cfg.Version}
	var out bytes.Buffer
	if err := fileTmpl.ExecuteTemplate(&out, tmplFile, data); err != nil {
		return err
	}
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		formatted = out.Bytes()
	}
	outPath := filepath.Join(absDir, cfg.Output)
	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		return err
	}
	return nil
}
