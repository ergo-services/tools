package templates

import (
	_ "embed"
	"text/template"
)

const (
	udpTemplatePoolFile   = "udp.tmpl"
	udpTemplateWorkerFile = "udp_worker.tmpl"
)

//go:embed udp.tmpl
var udpTemplatePoolText string

//go:embed udp_worker.tmpl
var udpTemplateWorkerText string

var UDP = []*template.Template{
	templateInit(udpTemplatePoolFile, udpTemplatePoolText),
	templateInit(udpTemplateWorkerFile, udpTemplateWorkerText),
}
