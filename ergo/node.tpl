package {{.Name}}

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	OptionNodeName   string
	OptionNodeCookie string
	{{ if .Cloud }}
	OptionCloudClusterName   string
	OptionCloudClusterCookie string
	{{ end }}
)

func init() {
	// generate random value for cookie
	buff := make([]byte, 12)
	rand.Read(buff)
	randomCookie = hex.EncodeToString(buff)

	flag.StringVar(&OptionNodeName, "name", "{{.Name}}@localhost", "node name")
	flag.StringVar(&OptionNodeCookie, "cookie", randomCookie, "a secret cookie for interaction within the cluster")
	{{ if .Cloud }}
	// cloud options
	flag.StringVar(&OptionCloudClusterName, "cloud-cluster", "{{.Cloud}}", "cloud cluster name")
	flag.StringVar(&OptionCloudClusterCookie, "cloud-cookie", "", "cloud cluster cookie")
	{{ end }}
}

func main() {
	var options node.Options

	flag.Parse()

	{{ if .Applications }}
	// create applications that must be started
	apps := []gen.ApplicationBehavior{ {{ range .Applications }}
	Create{{ . }}(), {{ end }}
	}
	options.Applications = apps
	{{ end }}

	{{ if .Cloud }}
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

	{{ if .RegisterTypes }}
	if err := registerTypes(); err != nil {
		panic(err)
	}
	{{ end }}

	{{ range .Processes }}
	// starting process {{ . }}
	process, err := {{$.Name}}Node.Spawn(strings.ToLower("{{ . }}"), gen.ProcessOptions{}, &{{ . }}{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("  process %q with PID %s is started\n", process.Name(), process.Self())
	{{ end }}

	{{.Name}}Node.Wait()
}
