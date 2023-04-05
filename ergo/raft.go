package main

import (
	_ "embed"
	"html/template"
)

const (
	raftTemplateFile = "raft.tpl"
)

//go:embed raft.tpl
var raftTemplateText string

var raftTemplates = []*template.Template{
	templateInit(raftTemplateFile, raftTemplateText),
}

type raftTemplateData struct {
	Package string
	Name    string
}
