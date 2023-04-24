package templates

import (
	_ "embed"
	"html/template"
)

const (
	tcpTemplateActorFile   = "tcp.tmpl"
	tcpTemplateHandlerFile = "tcp_handler.tmpl"
)

//go:embed tcp.tmpl
var tcpTemplateActorText string

//go:embed tcp_handler.tmpl
var tcpTemplateHandlerText string

var TCP = []*template.Template{
	templateInit(tcpTemplateActorFile, tcpTemplateActorText),
	templateInit(tcpTemplateHandlerFile, tcpTemplateHandlerText),
}
