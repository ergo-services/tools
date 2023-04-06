package {{ .Package }}

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

{{ range .Children -}}
type {{ .Name }} struct{
	// Add your fields
}
{{ end -}}

func RegisterTypes() error {
	types := []interface{}{
	{{ range .Children -}}
		{{ .Name }}{},
	{{ end -}}
	}

	opts := etf.RegisterTypeOptions{Strict: true}

	for _, t := range types {
		if _, err := etf.RegisterType(t, opts); err != nil && err != lib.ErrTaken {
			return err
		}
	}
	return nil
}
