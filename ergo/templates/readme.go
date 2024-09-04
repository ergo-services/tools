package templates

import (
	_ "embed"
	"text/template"
)

const (
	readmeTemplateFile = "readme.tmpl"
)

//go:embed readme.tmpl
var readmeTemplateText string

var Readme = []*template.Template{
	templateInit(readmeTemplateFile, readmeTemplateText),
}
