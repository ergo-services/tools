package main

import (
	_ "embed"
	"html/template"
)

const (
	sagaTemplateActorFile  = "saga.tpl"
	sagaTemplateWorkerFile = "saga_worker.tpl"
)

//go:embed saga.tpl
var sagaTemplateActorText string

//go:embed saga_worker.tpl
var sagaTemplateWorkerText string

var sagaTemplates = []*template.Template{
	templateInit(sagaTemplateActorFile, sagaTemplateActorText),
	templateInit(sagaTemplateWorkerFile, sagaTemplateWorkerText),
}

type sagaTemplateData struct {
	Package string
	Name    string
}
