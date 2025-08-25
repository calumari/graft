{{define "node_return"}}return {{$.Expr}}{{- if and $.WithError (not $.SuppressNil)}}, nil{{end}}{{end}}

{{define "node_unsupported"}}// unsupported mapping {{$.SrcType}} -> {{$.DestType}}{{end}}
