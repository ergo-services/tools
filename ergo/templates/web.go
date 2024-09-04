package templates

import (
	_ "embed"
	"text/template"
)

const (
	webTemplatePoolFile   = "web.tmpl"
	webTemplateWorkerFile = "web_worker.tmpl"
)

//go:embed web.tmpl
var webTemplatePoolText string

//go:embed web_worker.tmpl
var webTemplateWorkerText string

var Web = []*template.Template{
	templateInit(webTemplatePoolFile, webTemplatePoolText),
	templateInit(webTemplateWorkerFile, webTemplateWorkerText),
}
