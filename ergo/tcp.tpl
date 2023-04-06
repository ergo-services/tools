package {{ .Package }}

import (
	"crypto/tls"
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

var (
	enableTCPTLS = false
)

func create{{ .Name }}() gen.TCPBehavior {
	return &{{ .Name }}{}
}


type {{ .Name }} struct {
	gen.TCP
}

//
// Mandatory callbacks
//

// InitTCP
func (ts *{{ .Name }}) InitTCP(process *gen.TCPProcess, args ...etf.Term) (gen.TCPOptions, error) {
	options := gen.TCPOptions{
		Host:    "localhost",
		Port:    uint16(12345),
		Handler: create{{ .Name }}Handler(),
	}

	if enableTCPTLS {
		cert, _ := lib.GenerateSelfSignedCert("localhost")
		fmt.Println("TLS enabled. Generated self signed certificate. You may check it with command below:")
		fmt.Printf("   $ openssl s_client -connect %s:%d\n", options.Host, options.Port)
		options.TLS = &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
		}
	}

	return options, nil
}

//
// Optional gen.Server's callbacks
//

// HandleTCPCall this callback is invoked on ServerProcess.Call(...).
func (us *{{ .Name }}) HandleTCPCall(process *gen.TCPProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleTCPCast this callback is invoked on ServerProcess.Cast(...).
func (us *{{ .Name }}) HandleTCPCast(process *gen.TCPProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleTCPInfo this callback is invoked on Process.Send(...).
func (us *{{ .Name }}) HandleTCPInfo(process *gen.TCPProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleTCPTerminate this callback invoked on a process termination
func (us *{{ .Name }}) HandleTCPTerminate(process *gen.TCPProcess, reason string) {

}
