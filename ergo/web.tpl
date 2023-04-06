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
	enableWebTLS = false
)

func create{{ .Name }}() gen.WebBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Web
}

//
// Mandatory callbacks
//

// InitWeb invoked on starting Web server
func (w *{{ .Name }}) InitWeb(process *gen.WebProcess, args ...etf.Term) (gen.WebOptions, error) {
	var options gen.WebOptions

	options.Port = 8000
	options.Host = "localhost"
	proto := "http"

	if enableWebTLS {
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

//
// Optional gen.Server's callbacks
//

// HandleWebCall this callback is invoked on ServerProcess.Call(...).
func (w *{{ .Name }}) HandleWebCall(process *gen.WebProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleWebCast this callback is invoked on ServerProcess.Cast(...).
func (w *{{ .Name }}) HandleWebCast(process *gen.WebProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleWebInfo this callback is invoked on Process.Send(...).
func (w *{{ .Name }}) HandleWebInfo(process *gen.WebProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}
