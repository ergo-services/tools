package main

import (
	_ "embed"
	"html/template"
)

const (
	readmeTemplateFile = "readme.tpl"
)

//go:embed readme.tpl
var readmeTemplateText string

var readmeTemplates = []*template.Template{
	templateInit(readmeTemplateFile, readmeTemplateText),
}
