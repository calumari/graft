{{define "node_assignDirect"}}{{$.Dest}} = {{$.Src}}{{end}}

{{define "node_assignCast"}}{{$.Dest}} = {{$.CastType}}({{$.Src}})
{{end}}

{{define "node_assignHelper"}}{{if $.WithError }}tmp, err := {{$.Helper}}({{if $.UseContext}}ctx, {{end}}{{$.Src}})
if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
{{$.Dest}} = tmp
{{else}}{{$.Dest}} = {{$.Helper}}({{if $.UseContext}}ctx, {{end}}{{$.Src}})
{{end}}{{end}}

{{define "node_assignMethod"}}{{if $.WithError }}tmp, err := m.{{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}{{$.Arg}})
if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
{{$.Dest}} = tmp
{{else}}{{$.Dest}} = m.{{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}{{$.Arg}})
{{end}}{{end}}

{{define "node_assignFunc"}}{{if $.WithError}}tmp, err := {{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}{{$.Arg}})
if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
{{$.Dest}} = tmp
{{else}}{{$.Dest}} = {{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}{{$.Arg}})
{{end}}{{end}}
