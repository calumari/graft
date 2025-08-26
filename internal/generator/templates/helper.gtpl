{{define "helper"}}// {{.Name}} maps a value of type {{.SrcType}} to {{.DestType}}.
func {{.Name}}({{- if .HasContext}}ctx context.Context, {{end}}in {{.SrcType}}) {{- if .HasError}} ({{.DestType}}, error) {{- else}} {{.DestType}} {{- end}} {
    {{template "nodes" .Body}}
}
{{end}}
