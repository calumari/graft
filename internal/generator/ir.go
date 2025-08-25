package generator

// (no external deps needed here)

// This file houses the intermediate representation (IR) structures and enums
// used across generator phases (discovery -> modeling -> analysis -> render).

// node kinds for template-driven code emission
const (
	nodeKindComment      = "comment"
	nodeKindIfNilReturn  = "ifNilReturn"
	nodeKindDestInit     = "destInit"
	nodeKindAssignDirect = "assignDirect"
	nodeKindAssignCast   = "assignCast"
	nodeKindAssignHelper = "assignHelper"
	nodeKindAssignMethod = "assignMethod"
	nodeKindAssignFunc   = "assignFunc"
	nodeKindSliceMap     = "sliceMap"
	nodeKindArrayMap     = "arrayMap"
	nodeKindMapMap       = "mapMap"
	nodeKindPtrStructMap = "ptrStructMap"
	nodeKindPtrMethodMap = "ptrMethodMap"
	nodeKindPtrFuncMap   = "ptrFuncMap"
	nodeKindReturn       = "return"
	nodeKindUnsupported  = "unsupported"
)

// Config holds generation settings for the mapper generator.
type Config struct {
	Dir         string   // directory to load ("." relative to where command invoked)
	Interfaces  []string // interface type names to implement
	Output      string   // output filename
	CustomFuncs []string // optional: specific custom function names to consider (empty = discover all exported)
	Debug       bool     // when true, inject template debug comments linking nodes to templates
	Command     string   // full invocation command line
	Version     string   // graftgen build version
}

// fileModel is the root template model for a generated file.
type fileModel struct {
	Package     string
	Source      string
	Helpers     []helperModel
	Interfaces  []interfaceModel
	NeedContext bool
	Debug       bool
	Command     string
	Version     string
}

// interfaceModel describes a single interface mapping plan.
type interfaceModel struct {
	Name     string
	ImplName string
	Methods  []methodModel
}

// methodModel captures a single interface method mapping plan.
type methodModel struct {
	Name         string
	Params       []paramModel
	PrimaryParam string
	DestType     string
	HasError     bool
	Body         []codeNode
	HasContext   bool
}

// paramModel is a lightweight view of a method parameter for templates.
type paramModel struct {
	Name string
	Type string
}

// helperModel represents an internal helper function we synthesize for struct
// or collection mapping.
type helperModel struct {
	Name          string
	SrcType       string
	DestType      string
	Body          []codeNode
	HasError      bool
	HasContext    bool
	SrcIsPtr      bool
	DestIsPtr     bool
	UnderDestType string // underlying struct type when DestIsPtr
	ZeroReturn    string // zero literal used for early return on nil src when SrcIsPtr
}

// codeNode is an ir node used by templates to emit code fragments.
type codeNode struct {
	Kind          string
	Dest          string
	Src           string
	CastType      string
	Helper        string
	Method        string
	Arg           string
	Comment       string
	Var           string
	Zero          string
	WithError     bool
	Code          string
	DestType      string
	ElemType      string
	SrcType       string
	Expr          string
	Children      []codeNode
	LoopWithError bool
	UseContext    bool
	// debug fields
	Debug bool
	Path  string
}

// registryEntry consolidates previous methodMap/customFuncs into a single
// structure with an explicit kind for later specialization.
type registryEntry struct {
	Name     string
	HasError bool
	Kind     registryKind
	// For functions we may need to know quickly if it was originally custom.
}

type registryKind int

const (
	regKindInterfaceMethod registryKind = iota
	regKindCustomFunc
)
