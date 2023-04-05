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

func generate(option *Option) error {
	for _, t := range option.Templates {
		if err := os.MkdirAll(option.Dir, os.ModePerm); err != nil {
			return err
		}
		fmt.Println("LLL", option.IsApp, option.Name, option.Package, option.Dir)
		// template has format name.tpl or name_xxx.tpl
		// we need to compose file name like - <i.name>.go or <i.name>_<xxx>.go
		name := option.Name
		if _, xxx, found := strings.Cut(t.Name(), "_"); found {
			xxx, _ := strings.CutSuffix(xxx, ".tpl")
			name = name + "_" + xxx
		}
		file := strings.ToLower(path.Join(option.Dir, name+".go"))
		projectFile, err := os.Create(file)
		if err != nil {
			return err
		}
		defer projectFile.Close()

		fmt.Printf("   generating %q\n", file)
		buf, err := generateFile(t, option)
		if err != nil {
			return err
		}
		projectFile.Write(buf)
	}

	return nil
}

func generateFile(tmpl *template.Template, data any) ([]byte, error) {
	if tmpl == nil {
		panic("template is not initialized")
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, err
	}

	fmt.Println(data)
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
