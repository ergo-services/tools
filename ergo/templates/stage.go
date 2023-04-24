package templates

import (
	_ "embed"
	"html/template"
)

const (
	stageTemplateActorFile = "stage.tmpl"
)

//go:embed stage.tmpl
var stageTemplateActorText string

var Stage = []*template.Template{
	templateInit(stageTemplateActorFile, stageTemplateActorText),
}
