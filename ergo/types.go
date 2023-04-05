package main

import (
	"fmt"
	"html/template"
	"path"
	"strings"
)

var (
	optionsDict = make(map[string]*Option)
)

type listOptions []*Option
type Option struct {
	Name      string
	LoName    string
	Parent    *Option
	Children  []*Option
	Params    map[string]any
	Templates []*template.Template
	Dir       string
	Package   string
	IsApp     bool
}

func (l *listOptions) String() string {
	return ""
}

func (l *listOptions) Set(value string) error {
	var op Option

	value, hasOpts := strings.CutSuffix(value, "]")
	if hasOpts {
		s := strings.Split(value, "[")
		if len(s) != 2 {
			return fmt.Errorf("incorrect argument")
		}

		value = s[0]
		params, err := parseParams(s[1])
		if err != nil {
			return err
		}
		op.Params = params
	}

	s := strings.Split(value, ":")
	if len(s) > 2 {
		return fmt.Errorf("incorrect argument")
	}
	if len(s) == 2 {
		// has parent
		parent, exist := optionsDict[s[0]]
		if exist == false {
			return fmt.Errorf("unknown parent")
		}
		op.Parent = parent
		value = s[1]
	}
	op.Name = value
	op.LoName = strings.ToLower(value)
	op.Package = "main"
	optionsDict[op.Name] = &op
	if op.Parent != nil {
		op.Parent.Children = append(op.Parent.Children, &op)
	}

	*l = append(*l, &op)
	return nil
}

func (l *listOptions) WithTemplates(t []*template.Template) *listOptions {
	for _, option := range *l {
		option.Templates = t
	}
	return l
}

func (l *listOptions) WithAppDir(dir string) *listOptions {
	for _, option := range *l {
		option.Dir = path.Join(option.Dir, dir, option.LoName)
		option.Package = option.LoName
		option.IsApp = true
		for _, child := range option.Children {
			child.Dir = option.Dir
			child.Package = option.Package
			for _, c := range child.Children {
				c.Dir = option.Dir
				c.Package = option.Package
			}
		}
	}
	return l

}

func (l *listOptions) WithDir(dir string) *listOptions {
	for _, option := range *l {
		option.Dir = dir
		for _, child := range option.Children {
			child.Dir = option.Dir
		}
	}
	return l
}

func parseParams(p string) (map[string]any, error) {
	params := make(map[string]any)
	for _, pairs := range strings.Split(p, ",") {
		s := strings.Split(pairs, ":")
		if len(s) > 1 {
			params[s[0]] = s[1]
			continue
		}
		params[s[0]] = ""
		fmt.Println(pairs)
	}
	return params, nil
}
