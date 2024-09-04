package templates

import (
	_ "embed"
	"text/template"
)

const (
	typesTemplateFile = "types.tmpl"
)

//go:embed types.tmpl
var typesTemplateText string

var Types = []*template.Template{
	templateInit(typesTemplateFile, typesTemplateText),
}
