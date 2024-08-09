package templates

import (
	_ "embed"
	"text/template"
)

const (
	tcpTemplateActorFile = "tcp.tmpl"
)

//go:embed tcp.tmpl
var tcpTemplateActorText string

var TCP = []*template.Template{
	templateInit(tcpTemplateActorFile, tcpTemplateActorText),
}
