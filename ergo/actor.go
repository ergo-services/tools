package main

import (
	_ "embed"
	"html/template"
)

const (
	actorTemplateFile = "actor.tpl"
)

//go:embed actor.tpl
var actorTemplateText string

var actorTemplates = []*template.Template{
	templateInit(actorTemplateFile, actorTemplateText),
}
