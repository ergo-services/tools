package main

import (
	_ "embed"
	"html/template"
)

const (
	webTemplateActorFile   = "web.tpl"
	webTemplateHandlerFile = "web_handler.tpl"
)

//go:embed web.tpl
var webTemplateActorText string

//go:embed web_handler.tpl
var webTemplateHandlerText string

var webTemplates = []*template.Template{
	templateInit(webTemplateActorFile, webTemplateActorText),
	templateInit(webTemplateHandlerFile, webTemplateHandlerText),
}

type webTemplateData struct {
	Package string
	Name    string
}
