package templates

import (
	_ "embed"
	"html/template"
)

const (
	webTemplateActorFile   = "web.tmpl"
	webTemplateHandlerFile = "web_handler.tmpl"
)

//go:embed web.tmpl
var webTemplateActorText string

//go:embed web_handler.tmpl
var webTemplateHandlerText string

var Web = []*template.Template{
	templateInit(webTemplateActorFile, webTemplateActorText),
	templateInit(webTemplateHandlerFile, webTemplateHandlerText),
}
