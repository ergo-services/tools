package {{ .Package }}

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

var (
	enableTLS = false
}

func create{{ .Name }}() gen.ProcessBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Web
}

func (w *{{ .Name }}) InitWeb(process *gen.WebProcess, args ...etf.Term) (gen.WebOptions, error) {
	var options gen.WebOptions

	options.Port = 8000
	options.Host = "localhost"
	proto := "http"

	if enableTLS {
		// generate self-signed certificate
		cert, err := lib.GenerateSelfSignedCert("gen.Web - {{ .Name }}")
		if err != nil {
			return options, err
		}
		options.TLS = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		proto = "https"
	}

	mux := http.NewServeMux()
	whOptions := gen.WebHandlerOptions{
		NumHandlers:    50,
		IdleTimeout:    10,
		RequestTimeout: 20,
	}
	webRoot := process.StartWebHandler(create{{ .Name }}Handler(), whOptions)
	mux.Handle("/", webRoot)
	options.Handler = mux

	fmt.Printf("Start Web server on %s://%s:%d/\n", proto, options.Host, options.Port)
	return options, nil
}
