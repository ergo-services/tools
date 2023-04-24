package templates

import (
	_ "embed"
	"html/template"
)

const (
	poolTemplateActorFile  = "pool.tmpl"
	poolTemplateWorkerFile = "pool_worker.tmpl"
)

//go:embed pool.tmpl
var poolTemplateActorText string

//go:embed pool_worker.tmpl
var poolTemplateWorkerText string

var Pool = []*template.Template{
	templateInit(poolTemplateActorFile, poolTemplateActorText),
	templateInit(poolTemplateWorkerFile, poolTemplateWorkerText),
}
