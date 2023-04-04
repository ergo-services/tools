package main

import (
	_ "embed"
	"html/template"
)

const (
	stageTemplateFile = "stage.tpl"
)

//go:embed stage.tpl
var stageTemplateText string
var stageTemplate, _ = template.New(stageTemplateFile).Parse(stageTemplateText)
