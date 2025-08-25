{{define "helper"}}// Helper: {{.Name}}
func {{.Name}}({{- if .HasContext}}ctx context.Context, {{end}}in {{.SrcType}}) {{- if .HasError}} ({{.DestType}}, error) {{- else}} {{.DestType}} {{- end}} {
    // Destination zero value; fields populated by node sequence below.
    var dst {{.DestType}}
{{template "nodes" .Body}}
return dst{{- if .HasError}}, nil{{end}}
}
{{end}}
