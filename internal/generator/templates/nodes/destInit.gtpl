{{define "node_destInit"}}var {{.Var}} {{.DestType}}{{end}}

{{define "node_destInitAlloc"}}{{.Var}} := new({{.UnderType}}){{end}}
