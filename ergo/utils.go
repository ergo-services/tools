package main

import (
	"bytes"
	"fmt"
	format "go/format"
	"html/template"
	"os"
	"path"
)

func generateProject(dir string) error {
	file := path.Join(dir, root.name+".go")
	projectFile, err := os.Create(file)
	if err != nil {
		return err
	}
	defer projectFile.Close()

	//for _, child := range root.children {

	//}

	if buf, err := generate(root.tmpl, root.data); err == nil {
		fmt.Printf("   generating %q\n", file)
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
