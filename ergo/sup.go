package main

import (
	_ "embed"
	"html/template"
)

const (
	supTemplateFile = "sup.tpl"
)

//go:embed sup.tpl
var supTemplateText string

var supTemplates = []*template.Template{
	templateInit(supTemplateFile, supTemplateText),
}
