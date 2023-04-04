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
var nodeTemplate, nodeTemplateErr = template.New(nodeTemplateFile).Parse(nodeTemplateText)

type nodeTemplateData struct {
	Name  string
	Cloud string
}
