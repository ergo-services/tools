package {{ .Package }}

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type {{ .Name }} struct {
	gen.Pool
}

func create{{ .Name }}() gen.PoolBehavior {
	return &{{ .Name }}{}
}

func (p *{{ .Name }}) InitPool(process *gen.PoolProcess, args ...etf.Term) (gen.PoolOptions, error) {
	opts := gen.PoolOptions{
		Worker:     create{{ .Name }}Worker(),
		NumWorkers: 5,
	}

	return opts, nil
}
