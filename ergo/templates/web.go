package templates

import (
	_ "embed"
	"html/template"
)

const (
	webTemplateActorFile = "web.tmpl"
)

//go:embed web.tmpl
var webTemplateActorText string

var Web = []*template.Template{
	templateInit(webTemplateActorFile, webTemplateActorText),
}
