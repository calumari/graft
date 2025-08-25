{{/* Insert newlines only between nodes (avoids trailing newline) */}}
{{define "nodes"}}{{range $i, $e := .}}{{if gt $i 0}}
{{end}}{{template "node" $e}}{{end}}{{end}}

{{define "node"}}{{if .Debug }}// node START {{.Path}} kind={{.Kind}}
{{end}}{{- if eq .Kind "comment" -}}
    {{template "node_comment" .}}
{{- else if eq .Kind "ifNilReturn" -}}
    {{template "node_ifNilReturn" .}}
{{- else if eq .Kind "destInit" -}}
    {{template "node_destInit" .}}
{{- else if eq .Kind "assignDirect" -}}
    {{template "node_assignDirect" .}}
{{- else if eq .Kind "assignCast" -}}
    {{template "node_assignCast" .}}
{{- else if eq .Kind "assignHelper" -}}
    {{template "node_assignHelper" .}}
{{- else if eq .Kind "assignMethod" -}}
    {{template "node_assignMethod" .}}
{{- else if eq .Kind "assignFunc" -}}
    {{template "node_assignFunc" .}}
{{- else if eq .Kind "sliceMap" -}}
    {{template "node_sliceMap" .}}
{{- else if eq .Kind "arrayMap" -}}
    {{template "node_arrayMap" .}}
{{- else if eq .Kind "mapMap" -}}
    {{template "node_mapMap" .}}
{{- else if eq .Kind "ptrStructMap" -}}
    {{template "node_ptrStructMap" .}}
{{- else if eq .Kind "ptrMethodMap" -}}
    {{template "node_ptrMethodMap" .}}
{{- else if eq .Kind "ptrFuncMap" -}}
    {{template "node_ptrFuncMap" .}}
{{- else if eq .Kind "return" -}}
    {{template "node_return" .}}
{{- else if eq .Kind "unsupported" -}}
    {{template "node_unsupported" .}}
{{end}}{{if .Debug}}// node END {{.Path}}{{end}}{{end}}