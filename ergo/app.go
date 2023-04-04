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
var appTemplate, _ = template.New(appTemplateFile).Parse(appTemplateText)
