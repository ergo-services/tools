package main

import (
	_ "embed"
	"html/template"
)

const (
	tcpTemplateActorFile   = "tcp.tpl"
	tcpTemplateHandlerFile = "tcp_handler.tpl"
)

//go:embed tcp.tpl
var tcpTemplateActorText string

//go:embed tcp_handler.tpl
var tcpTemplateHandlerText string

var tcpTemplates = []*template.Template{
	templateInit(tcpTemplateActorFile, tcpTemplateActorText),
	templateInit(tcpTemplateHandlerFile, tcpTemplateHandlerText),
}

type tcpTemplateData struct {
	Package string
	Name    string
}
