{{define "node_ifNilReturn"}}if {{$.Var}} == nil {
	return {{$.Zero}}{{- if $.WithError}}, nil{{end}}
}{{end}}
