package templates

import (
	_ "embed"
	"html/template"
)

const (
	nodeTemplateFile = "node.tmpl"
)

//go:embed node.tmpl
var nodeTemplateText string
var Node = []*template.Template{
	templateInit(nodeTemplateFile, nodeTemplateText),
}
