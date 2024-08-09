package templates

import (
	_ "embed"
	"text/template"
)

const (
	nodeTemplateFile = "node.tmpl"
)

//go:embed node.tmpl
var nodeTemplateText string
var Node = []*template.Template{
	templateInit(nodeTemplateFile, nodeTemplateText),
}
