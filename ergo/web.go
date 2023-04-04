package main

import (
	_ "embed"
	"html/template"
)

const (
	webTemplateFile = "web.tpl"
)

//go:embed web.tpl
var webTemplateText string
var webTemplate, _ = template.New(webTemplateFile).Parse(webTemplateText)
