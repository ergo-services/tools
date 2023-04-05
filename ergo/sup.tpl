package {{ .Package }}

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.SupervisorBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Supervisor
}

func (sup *{{ .Name }}) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	spec := gen.SupervisorSpec{
		Name: "{{ .LoName }}",
		Children: []gen.SupervisorChildSpec{ {{ range .Children }}
			gen.SupervisorChildSpec{
				Name:  "{{ .LoName }}",
				Child: create{{ .Name }}(),
			},
		{{ end }} },
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategyOneForAll,
			Intensity: 2,
			Period:    5,
			Restart:   gen.SupervisorStrategyRestartTemporary,
		},
	}
	return spec, nil
}
