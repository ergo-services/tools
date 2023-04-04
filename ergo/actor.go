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
var actorTemplate, _ = template.New(actorTemplateFile).Parse(actorTemplateText)
