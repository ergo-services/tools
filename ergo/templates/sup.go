package templates

import (
	_ "embed"
	"html/template"
)

const (
	supTemplateFile = "sup.tmpl"
)

//go:embed sup.tmpl
var supTemplateText string

var Sup = []*template.Template{
	templateInit(supTemplateFile, supTemplateText),
}
