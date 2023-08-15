package templates

import (
	_ "embed"
	"html/template"
)

const (
	udpTemplateActorFile = "udp.tmpl"
)

//go:embed udp.tmpl
var udpTemplateActorText string

var UDP = []*template.Template{
	templateInit(udpTemplateActorFile, udpTemplateActorText),
}
