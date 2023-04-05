package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func Create{{ .Name }}() gen.ApplicationBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Application
}

func (app *{{ .Name }}) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "{{ .LoName }}",
		Description: "description of this application",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{ {{ range .Children }}
			gen.ApplicationChildSpec{
				Name:  "{{ .LoName }}",
				Child: create{{ .Name }}(),
			},
		{{ end }} },
	}, nil
}

func (app *{{ .Name }}) Start(process gen.Process, args ...etf.Term) {
	fmt.Printf("Application {{ .Name }} started with Pid %s\n", process.Self())
}
