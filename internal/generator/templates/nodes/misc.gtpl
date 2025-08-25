{{define "node_return"}}return {{$.Expr}}{{- if $.WithError}}, nil{{end}}{{end}}

{{define "node_unsupported"}}// unsupported mapping {{$.SrcType}} -> {{$.DestType}}{{end}}
