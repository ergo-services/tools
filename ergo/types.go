package main

import "html/template"

type listOptions []string

func (l *listOptions) String() string {
	return ""
}

func (l *listOptions) Set(value string) error {
	*l = append(*l, value)
	return nil
}

type item struct {
	app      string
	name     string
	tmpl     []*template.Template
	data     any
	children []*item
}

type actor struct {
	list listOptions
	tmpl []*template.Template
}
