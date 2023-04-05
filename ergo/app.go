package main

import (
	_ "embed"
	"html/template"
)

const (
	appTemplateFile = "app.tpl"
)

//go:embed app.tpl
var appTemplateText string

var appTemplates = []*template.Template{
	templateInit(appTemplateFile, appTemplateText),
}

type appTemplateData struct {
	Package  string
	Name     string
	Children []string
}
