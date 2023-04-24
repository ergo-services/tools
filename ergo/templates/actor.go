package templates

import (
	_ "embed"
	"html/template"
)

const (
	actorTemplateFile = "actor.tmpl"
)

//go:embed actor.tmpl
var actorTemplateText string

var Actor = []*template.Template{
	templateInit(actorTemplateFile, actorTemplateText),
}
