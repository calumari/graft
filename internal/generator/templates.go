package generator

import (
	"embed"
	"fmt"
	"sync"
	"text/template"
)

const (
	tmplFile      = "file"
	tmplHelper    = "helper"
	tmplInterface = "interface"
	tmplNodes     = "nodes"
	tmplNode      = "node"

	tmplNodeComment      = "comment"
	tmplNodeIfNilReturn  = "ifNilReturn"
	tmplNodeDestInit     = "destInit"
	tmplNodeAssignDirect = "assignDirect"
	tmplNodeAssignCast   = "assignCast"
	tmplNodeAssignHelper = "assignHelper"
	tmplNodeAssignMethod = "assignMethod"
	tmplNodeAssignFunc   = "assignFunc"
	tmplNodeSliceMap     = "sliceMap"
	tmplNodeArrayMap     = "arrayMap"
	tmplNodeMapMap       = "mapMap"
	tmplNodePtrStructMap = "ptrStructMap"
	tmplNodePtrMethodMap = "ptrMethodMap"
	tmplNodePtrFuncMap   = "ptrFuncMap"
	tmplNodeReturn       = "return"
	tmplNodeUnsupported  = "unsupported"
)

const (
	templatePattern      = "templates/*.gtpl"
	templateNodesPattern = "templates/nodes/*.gtpl"
)

//go:embed templates/*.gtpl templates/nodes/*.gtpl
var templatesFS embed.FS

var (
	fileTmpl     *template.Template
	tmplInitOnce sync.Once
	tmplInitErr  error
)

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

	// Validate node_* templates for every node kind constant (keeps dispatch.gtpl in sync with IR).
	requiredNodeKinds := []string{
		tmplNodeComment,
		tmplNodeIfNilReturn,
		tmplNodeDestInit,
		tmplNodeAssignDirect,
		tmplNodeAssignCast,
		tmplNodeAssignHelper,
		tmplNodeAssignMethod,
		tmplNodeAssignFunc,
		tmplNodeSliceMap,
		tmplNodeArrayMap,
		tmplNodeMapMap,
		tmplNodePtrStructMap,
		tmplNodePtrMethodMap,
		tmplNodePtrFuncMap,
		tmplNodeReturn,
		tmplNodeUnsupported,
	}
	for _, kind := range requiredNodeKinds {
		name := "node_" + kind
		if fileTmpl.Lookup(name) == nil {
			return fmt.Errorf("required node template %q for kind %q not found", name, kind)
		}
	}
	return nil
}

// ensureTemplates parses and validates templates exactly once.
func ensureTemplates() error {
	tmplInitOnce.Do(func() {
		var t *template.Template
		t, tmplInitErr = template.New(tmplFile).ParseFS(templatesFS, templatePattern, templateNodesPattern)
		if tmplInitErr != nil {
			return
		}
		fileTmpl = t
		tmplInitErr = validateTemplates()
	})
	return tmplInitErr
}
