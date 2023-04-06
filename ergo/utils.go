package main

import (
	"bytes"
	"fmt"
	format "go/format"
	"html/template"
	"os"
	"os/exec"
	"path"
	"strings"
)

func generate(option *Option) error {
	for _, t := range option.Templates {
		dir := option.Dir
		if option.Package == "main" {
			dir = path.Join(dir, "cmd")
		}
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
		// template has format name.tpl or name_xxx.tpl
		// we need to compose file name like - <i.name>.go or <i.name>_<xxx>.go
		name := option.Name
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

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func generateGoMod(option *Option) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(option.Dir); err != nil {
		return err
	}
	fmt.Printf("   generating %q\n", "go.mod")
	cmd := exec.Command("go", "mod", "init", option.Name)
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("   generating %q\n", "go.sum")
	cmd = exec.Command("go", "mod", "tidy")
	if err := cmd.Run(); err != nil {
		return err
	}
	os.Chdir(currentDir)
	return nil
}

func templateInit(name string, text string) *template.Template {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		fmt.Println("error: can't initialize template")
		panic(err)
	}
	return tmpl
}
