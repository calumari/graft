{{define "node_ptrStructMap"}}if {{$.Src}} != nil {
    {{- if $.WithError }}
    tmp, err := {{$.Helper}}({{if $.UseContext}}ctx, {{end}}*{{$.Src}})
    if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
    {{$.Dest}} = &tmp
    {{- else }}
    tmp := {{$.Helper}}({{if $.UseContext}}ctx, {{end}}*{{$.Src}})
    {{$.Dest}} = &tmp
    {{- end }}
} else {
    {{$.Dest}} = nil
}{{end}}

{{define "node_ptrMethodMap"}}if {{$.Src}} != nil {
    {{- if $.WithError }}
    tmp, err := m.{{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}*{{$.Src}})
    if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
    {{$.Dest}} = &tmp
    {{- else }}
    tmp := m.{{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}*{{$.Src}})
    {{$.Dest}} = &tmp
    {{- end }}
} else {
    {{$.Dest}} = nil
}{{end}}

{{define "node_ptrFuncMap"}}if {{$.Src}} != nil {
    {{- if $.WithError }}
    tmp, err := {{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}*{{$.Src}})
    if err != nil { return {{if eq $.Dest "mapped"}}dst{{else}}{{$.Dest}}{{end}}, err }
    {{$.Dest}} = &tmp
    {{- else }}
    tmp := {{$.Method}}({{if $.UseContext}}{{$.CtxName}}, {{end}}*{{$.Src}})
    {{$.Dest}} = &tmp
    {{- end }}
} else {
    {{$.Dest}} = nil
}{{end}}
