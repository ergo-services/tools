package main

import (
	_ "embed"
	"html/template"
)

const (
	stageTemplateActorFile      = "stage.tpl"
	stageTemplateDispatcherFile = "stage_dispatcher.tpl"
)

//go:embed stage.tpl
var stageTemplateActorText string

//go:embed stage_dispatcher.tpl
var stageTemplateDispatcherText string

var stageTemplates = []*template.Template{
	templateInit(stageTemplateActorFile, stageTemplateActorText),
	templateInit(stageTemplateDispatcherFile, stageTemplateDispatcherText),
}

type stageTemplateData struct {
	Package string
	Name    string
}
