package main

import (
	_ "embed"
	"html/template"
)

const (
	sagaTemplateFile = "saga.tpl"
)

//go:embed saga.tpl
var sagaTemplateText string
var sagaTemplate, _ = template.New(sagaTemplateFile).Parse(sagaTemplateText)
