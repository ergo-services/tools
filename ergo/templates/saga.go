package templates

import (
	_ "embed"
	"html/template"
)

const (
	sagaTemplateActorFile  = "saga.tmpl"
	sagaTemplateWorkerFile = "saga_worker.tmpl"
)

//go:embed saga.tmpl
var sagaTemplateActorText string

//go:embed saga_worker.tmpl
var sagaTemplateWorkerText string

var Saga = []*template.Template{
	templateInit(sagaTemplateActorFile, sagaTemplateActorText),
	templateInit(sagaTemplateWorkerFile, sagaTemplateWorkerText),
}
