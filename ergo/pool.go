package main

import (
	_ "embed"
	"html/template"
)

const (
	poolTemplateFile = "pool.tpl"
)

//go:embed pool.tpl
var poolTemplateText string
var poolTemplate, _ = template.New(poolTemplateFile).Parse(poolTemplateText)
