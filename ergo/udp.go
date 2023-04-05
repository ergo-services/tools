package main

import (
	_ "embed"
	"html/template"
)

const (
	udpTemplateActorFile   = "udp.tpl"
	udpTemplateHandlerFile = "udp_handler.tpl"
)

//go:embed udp.tpl
var udpTemplateActorText string

//go:embed udp_handler.tpl
var udpTemplateHandlerText string

var udpTemplates = []*template.Template{
	templateInit(udpTemplateActorFile, udpTemplateActorText),
	templateInit(udpTemplateHandlerFile, udpTemplateHandlerText),
}

type udpTemplateData struct {
	Package string
	Name    string
}
