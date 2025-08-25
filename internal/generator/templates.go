package generator

import (
	"embed"
	"fmt"
	"text/template"
)

const (
	tmplFile      = "file"
	tmplHelper    = "helper"
	tmplInterface = "interface"
	tmplNodes     = "nodes"
	tmplNode      = "node"
)

const (
	templatePattern      = "templates/*.gtpl"
	templateNodesPattern = "templates/nodes/*.gtpl"
)

//go:embed templates/*.gtpl templates/nodes/*.gtpl
var templatesFS embed.FS

// fileTmpl is the parsed master template assembled from modular template files
var fileTmpl = template.Must(template.New(tmplFile).ParseFS(templatesFS, templatePattern, templateNodesPattern))

// validateTemplates ensures all required templates are defined
func validateTemplates() error {
	requiredTemplates := []string{
		tmplFile,
		tmplHelper,
		tmplInterface,
		tmplNodes,
		tmplNode,
	}

	for _, name := range requiredTemplates {
		if fileTmpl.Lookup(name) == nil {
			return fmt.Errorf("required template %q not found", name)
		}
	}
	return nil
}

func init() {
	if err := validateTemplates(); err != nil {
		panic(fmt.Sprintf("template validation failed: %v", err))
	}
}
