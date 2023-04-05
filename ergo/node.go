package main

import (
	_ "embed"
	"html/template"
)

const (
	nodeTemplateFile = "node.tpl"
)

//go:embed node.tpl
var nodeTemplateText string
var nodeTemplates = []*template.Template{
	templateInit(nodeTemplateFile, nodeTemplateText),
}
