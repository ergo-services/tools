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
		file := path.Join(dir, option.Name)
		if option.KeepOriginalName == false {
			// template has format name.tpl or name_xxx.tpl
			// we need to compose file name like - <t.Name>.go or <t.Name>_<xxx>.go
			name := option.Name
			if _, xxx, found := strings.Cut(t.Name(), "_"); found {
				xxx, _ := strings.CutSuffix(xxx, ".tmpl")
				name = name + "_" + xxx
			}
			file = strings.ToLower(path.Join(dir, name+".go"))
		}
		projectFile, err := os.Create(file)
		if err != nil {
			return err
		}
		defer projectFile.Close()

		fmt.Printf("   generating %q\n", file)
		buf, err := generateFile(t, option, option.SkipGoFormat)
		if err != nil {
			return err
		}
		projectFile.Write(buf)
	}

	return nil
}

func generateFile(tmpl *template.Template, data any, skipGoFormat bool) ([]byte, error) {
	if tmpl == nil {
		panic("template is not initialized")
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, err
	}

	if skipGoFormat {
		return buf.Bytes(), nil
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Println("---")
		fmt.Println(buf.String())
		fmt.Println("---")
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
	fmt.Printf("   generating %q\n", path.Join(option.Dir, "go.mod"))
	cmd := exec.Command("go", "mod", "init", option.Params["module"].(string))
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("   generating %q\n", path.Join(option.Dir, "go.sum"))
	cmd = exec.Command("go", "mod", "tidy")
	if err := cmd.Run(); err != nil {
		return err
	}
	os.Chdir(currentDir)
	return nil
}
