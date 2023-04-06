package main

import (
	_ "embed"
	"html/template"
)

const (
	typesTemplateFile = "types.tpl"
)

//go:embed types.tpl
var typesTemplateText string

var typesTemplates = []*template.Template{
	templateInit(typesTemplateFile, typesTemplateText),
}
