## Project: "{{ index .Params "project" }}"

### Generated with
 - Types for network messaging: {{ index .Params "optionTypes" }}
 - Enabled Cloud feature: {{ index .Params "optionCloud"}}

### Supervision Tree

{{ if index .Params "applications" }}
Applications
{{ range index .Params "applications" }} - `{{ .Name }}{}` {{ .Dir }}/{{ .LoName }}.go
  {{ range .Children }} - `{{ .Name }}{}` {{ .Dir }}/{{ .LoName }}.go
    {{ range .Children }} - `{{ .Name }}{}` {{ .Dir }}/{{ .LoName }}.go
    {{ end }}
  {{ end }}
 {{ end }}
{{ end }}
{{ if index .Params "processes" }}
Process list that is starting by node directly
{{ range index .Params "processes" }} - `{{ .Name }}{}` {{ .Dir }}/cmd/{{ .LoName }}.go
  {{ range .Children }} - `{{ .Name }}{}` {{ .Dir }}/{{ .LoName }}.go
  {{ end }}
{{ end }}
{{ if index .Params "types" }}
Messages are generated for the networking in {{ .Dir }}/types.go
{{ range index .Params "types" }} - `{{ .Name }}{}`
{{ end }}
{{ end }}
{{ end }}
