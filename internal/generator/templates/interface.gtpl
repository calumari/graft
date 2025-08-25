{{define "interface"}}// {{.ImplName}} is the generated implementation of {{.Name}}.
type {{.ImplName}} struct{}

// New{{.Name}} returns a new {{.Name}} implementation.
func New{{.Name}}() {{.Name}} { return &{{.ImplName}}{} }

{{range .Methods}}
// {{.Name}} maps {{.PrimaryParam}} to the destination type.
func (m *{{$.ImplName}}) {{.Name}}({{range $i, $p := .Params}}{{if $i}}, {{end}}{{$p.Name}} {{$p.Type}}{{end}}) {{- if .HasError}}({{.DestType}}, error){{else}} {{.DestType}}{{end}} {
{{template "nodes" .Body}}
}
{{end}}
{{end}}
