package main

import (
	_ "embed"
	"html/template"
)

const (
	stageTemplateActorFile = "stage.tpl"
)

//go:embed stage.tpl
var stageTemplateActorText string

var stageTemplates = []*template.Template{
	templateInit(stageTemplateActorFile, stageTemplateActorText),
}
