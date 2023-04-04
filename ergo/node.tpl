package main

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
	flag.StringVar(&OptionCloudClusterName, "cloud-cluster", "{{.Cloud}}", "cloud cluster name")
	flag.StringVar(&OptionCloudClusterCookie, "cloud-cookie", "", "cloud cluster cookie")
	{{ end }}
}

func main() {
	var options node.Options

	flag.Parse()

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

	fmt.Println("Start node %q\n", OptionNodeName)
	{{.Name}}Node, _ := ergo.StartNode(OptionNodeName, OptionNodeCookie, options)

	process, _ := myNode.Spawn("simple", gen.ProcessOptions{}, &simple{})
	fmt.Printf("Started process %s with name %q\n", process.Self(), process.Name())

	{{.Name}}Node.Wait()
}
