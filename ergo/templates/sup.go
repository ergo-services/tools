package templates

import (
	_ "embed"
	"text/template"
)

const (
	supTemplateFile = "sup.tmpl"
)

//go:embed sup.tmpl
var supTemplateText string

var Sup = []*template.Template{
	templateInit(supTemplateFile, supTemplateText),
}
