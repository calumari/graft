{{define "node_sliceMap"}}if {{$.Src}} != nil {
    {{$.Dest}} = make({{$.DestType}}, len({{$.Src}}))
    for i, v := range {{$.Src}} { // v used by child nodes
        var mapped {{$.ElemType}}
{{template "nodes" $.Children}}
        {{$.Dest}}[i] = mapped
    }
} else {
    {{$.Dest}} = nil
}{{end}}

{{define "node_arrayMap"}}for i := range {{$.Src}} {
{{template "nodes" $.Children}}
}{{end}}

{{define "node_mapMap"}}if {{$.Src}} != nil {
    {{- $.Dest}} = make({{$.DestType}}, len({{$.Src}}))
    for k, v := range {{$.Src}} { // k,v used by child nodes
        var mapped {{$.ElemType}}
{{template "nodes" $.Children}}
        {{$.Dest}}[k] = mapped
    }
} else {
    {{$.Dest}} = nil
}{{end}}
