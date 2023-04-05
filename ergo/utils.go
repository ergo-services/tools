package main

import (
	"bytes"
	"fmt"
	format "go/format"
	"html/template"
	"os"
	"path"
	"strings"
)

func generateProject(i *item, dir string) error {
	for _, child := range i.children {
		childDir := path.Join(dir, child.app)
		if err := generateProject(child, childDir); err != nil {
			return err
		}
	}

	for _, t := range i.tmpl {
		// template has format name.tpl or name_xxx.tpl
		// we need to compose file name like - <i.name>.go or <i.name>_<xxx>.go
		name := i.name
		if _, xxx, found := strings.Cut(t.Name(), "_"); found {
			xxx, _ := strings.CutSuffix(xxx, ".tpl")
			name = name + "_" + xxx
		}
		file := strings.ToLower(path.Join(dir, name+".go"))
		projectFile, err := os.Create(file)
		if err != nil {
			return err
		}
		defer projectFile.Close()

		fmt.Printf("   generating %q\n", file)
		buf, err := generate(t, i.data)
		if err != nil {
			return err
		}
		projectFile.Write(buf)
	}
	return nil
}

func generate(tmpl *template.Template, data any) ([]byte, error) {
	if tmpl == nil {
		panic("template is not initialized")
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func templateInit(name string, text string) *template.Template {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		fmt.Println("error: can't initialize template")
		panic(err)
	}
	return tmpl
}
