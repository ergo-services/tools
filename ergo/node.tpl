package {{.Package}}

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	{{ range index .Params "applications" }} "{{ $.Name }}/apps/{{ .LoName }}"
	{{ end }}
	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	OptionNodeName   string
	OptionNodeCookie string
	{{ if index .Params "cloud" }}
	OptionCloudClusterName   string
	OptionCloudClusterCookie string
	{{ end }}
)

func init() {
	// generate random value for cookie
	buff := make([]byte, 12)
	rand.Read(buff)
	randomCookie := hex.EncodeToString(buff)

	flag.StringVar(&OptionNodeName, "name", "{{.Name}}@localhost", "node name")
	flag.StringVar(&OptionNodeCookie, "cookie", randomCookie, "a secret cookie for interaction within the cluster")
	{{ if index .Params "cloud" }}
	// cloud options
	flag.StringVar(&OptionCloudClusterName, "cloud-cluster", "{{ index .Params "cloud" }}", "cloud cluster name")
	flag.StringVar(&OptionCloudClusterCookie, "cloud-cookie", "", "cloud cluster cookie")
	{{ end }}
}

func main() {
	var options node.Options
	var process gen.Process

	flag.Parse()

	{{ if index .Params "applications" }}
	// create applications that must be started
	apps := []gen.ApplicationBehavior{ {{ range index .Params "applications" }}
	{{ .LoName }}.Create{{ .Name }}(), {{ end }}
	}
	options.Applications = apps
	{{ end }}

	{{ if index .Params "cloud" }}
	// Enable cloud feature.
	options.Cloud.Enable = true

	// Set your cluster name and cookie to get access to the cloud
	options.Cloud.Cluster = OptionCloudClusterName
	options.Cloud.Cookie = OptionCloudClusterCookie

	// We should enable accepting incoming connection requests
	// from the nodes in your cloud cluster.
	options.Proxy.Accept = true
	{{ end }}

	// starting node
	{{.Name}}Node, err := ergo.StartNode(OptionNodeName, OptionNodeCookie, options)
	if err != nil {
		panic(err)
	}
	fmt.Printf("node %q is started\n", {{.Name}}Node.Name())

	{{ if index .Params "register" }}
	//if err := registerTypes(); err != nil {
	//	panic(err)
	//}
	{{ end }}

	{{ range .Children}}
	// starting process {{ .Name }}
	process, err = {{$.Name}}Node.Spawn("{{ .LoName }}", gen.ProcessOptions{}, &{{ .Name }}{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("  process %q with PID %s is started\n", process.Name(), process.Self())
	{{ end }}

	{{.Name}}Node.Wait()
}
