package templates

import (
	_ "embed"
	"html/template"
)

const (
	raftTemplateFile = "raft.tmpl"
)

//go:embed raft.tmpl
var raftTemplateText string

var Raft = []*template.Template{
	templateInit(raftTemplateFile, raftTemplateText),
}
