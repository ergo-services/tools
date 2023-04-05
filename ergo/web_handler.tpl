package {{ .Package }}

import (
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}Handler() gen.WebHandlerBehavior {
	return &{{ .Name }}Handler{}
}

type {{ .Name }}Handler struct {
	gen.WebHandler
}

func (r *{{ .Name }}Handler) HandleRequest(process *gen.WebHandlerProcess, request gen.WebMessageRequest) gen.WebHandlerStatus {
	request.Response.Write([]byte("Hello"))
	return gen.WebHandlerStatusDone
}
