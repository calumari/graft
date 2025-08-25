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
	g.helperNames = make(map[string]string)
	g.helperModels = nil
	funcMap := g.discoverCustomFuncs(pkg, cfg.CustomFuncs)
	for k, mi := range funcMap {
		g.registry[k] = mi
		if !mi.HasError {
			base := strings.TrimSuffix(k, "#err")
			if _, ok := g.registry[base]; !ok {
				g.registry[base] = registryEntry{Name: mi.Name, HasError: false, Kind: regKindCustomFunc}
			}
		}
	}
	var interfaceModels []interfaceModel
	allPlans := make([][]methodPlan, 0, len(cfg.Interfaces))
	for _, name := range cfg.Interfaces {
		model, plans, err := g.buildInterfaceModel(name, ifaceMap[name])
		if err != nil {
			return err
		}
		interfaceModels = append(interfaceModels, *model)
		allPlans = append(allPlans, plans)
	}

	// Populate helpers discovered during method planning (single-param struct, composite)
	g.populateHelpers(pkg.Types.Scope())
	// Build method bodies from plans (may add helper plans during assignment building)
	for i := range interfaceModels {
		if err := g.populateMethods(&interfaceModels[i], allPlans[i]); err != nil {
			return err
		}
	}
	// Populate any newly created helper plans
	g.populateHelpers(pkg.Types.Scope())
	// De-duplicate helpers by name (defensive against accidental double population)
	{
		seen := map[string]bool{}
		uniq := make([]helperModel, 0, len(g.helperModels))
		for _, h := range g.helperModels {
			if !seen[h.Name] {
				seen[h.Name] = true
				uniq = append(uniq, h)
			}
		}
		g.helperModels = uniq
	}
	// Analyze helper error propagation now
	helperErrMap := g.computeHelperErrors()
	g.annotateHelperErrorUsage(&interfaceModels, helperErrMap)
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
