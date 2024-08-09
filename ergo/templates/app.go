package templates

import (
	_ "embed"
	"text/template"
)

const (
	appTemplateFile = "app.tmpl"
)

//go:embed app.tmpl
var appTemplateText string

var App = []*template.Template{
	templateInit(appTemplateFile, appTemplateText),
}
