package templates

import (
	_ "embed"
	"html/template"
)

const (
	udpTemplateActorFile   = "udp.tmpl"
	udpTemplateHandlerFile = "udp_handler.tmpl"
)

//go:embed udp.tmpl
var udpTemplateActorText string

//go:embed udp_handler.tmpl
var udpTemplateHandlerText string

var UDP = []*template.Template{
	templateInit(udpTemplateActorFile, udpTemplateActorText),
	templateInit(udpTemplateHandlerFile, udpTemplateHandlerText),
}
