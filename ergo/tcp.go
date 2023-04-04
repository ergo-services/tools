package main

import (
	_ "embed"
	"html/template"
)

const (
	tcpTemplateFile = "tcp.tpl"
)

//go:embed tcp.tpl
var tcpTemplateText string
var tcpTemplate, _ = template.New(tcpTemplateFile).Parse(tcpTemplateText)
