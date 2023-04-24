package templates

import (
	"fmt"
	"html/template"
)

func templateInit(name string, text string) *template.Template {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		fmt.Println("error: can't initialize template")
		panic(err)
	}
	return tmpl
}
