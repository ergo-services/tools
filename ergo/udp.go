package main

import (
	_ "embed"
	"html/template"
)

const (
	udpTemplateFile = "udp.tpl"
)

//go:embed udp.tpl
var udpTemplateText string
var udpTemplate, _ = template.New(udpTemplateFile).Parse(udpTemplateText)
