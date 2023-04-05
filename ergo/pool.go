package main

import (
	_ "embed"
	"html/template"
)

const (
	poolTemplateActorFile  = "pool.tpl"
	poolTemplateWorkerFile = "pool_worker.tpl"
)

//go:embed pool.tpl
var poolTemplateActorText string

//go:embed pool_worker.tpl
var poolTemplateWorkerText string

var poolTemplates = []*template.Template{
	templateInit(poolTemplateActorFile, poolTemplateActorText),
	templateInit(poolTemplateWorkerFile, poolTemplateWorkerText),
}
