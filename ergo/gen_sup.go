package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"text/template"
)

// supData is passed to supervisor templates.
type supData struct {
	Package   string
	Name      string
	SupType   string
	Strategy  string
	Intensity int
	Period    int
	Children  []ChildSpec
}

// genSup generates the _gen.go and user-owned .go files for a supervisor,
// then recursively generates all its children.
func genSup(tmplSet *template.Template, outputDir string, pkg string, child *ChildSpec) error {
	name := child.Sup
	lname := strings.ToLower(name)

	intensity := child.Intensity
	if intensity == 0 {
		intensity = 2
	}
	period := child.Period
	if period == 0 {
		period = 5
	}

	data := supData{
		Package:   pkg,
		Name:      name,
		SupType:   supType(child.Type),
		Strategy:  supStrategy(child.Strategy),
		Intensity: intensity,
		Period:    period,
		Children:  child.Children,
	}

	// _gen.go
	var genBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&genBuf, "sup_gen.tmpl", data); err != nil {
		return err
	}
	genPath := filepath.Join(outputDir, lname+"_gen.go")
	if err := writeGenFile(genPath, genBuf.Bytes()); err != nil {
		return err
	}

	// user-owned .go
	var userBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&userBuf, "sup_user.tmpl", data); err != nil {
		return err
	}
	userPath := filepath.Join(outputDir, lname+".go")
	if err := writeUserFile(userPath, userBuf.Bytes()); err != nil {
		return err
	}

	// recurse into children
	for i := range child.Children {
		c := &child.Children[i]
		if err := genChild(tmplSet, outputDir, pkg, c); err != nil {
			return err
		}
	}
	return nil
}

// genChild dispatches generation for a single ChildSpec.
func genChild(tmplSet *template.Template, outputDir string, pkg string, child *ChildSpec) error {
	if child.IsSup() == true {
		return genSup(tmplSet, outputDir, pkg, child)
	}
	if child.IsActor() == true {
		return genActor(tmplSet, outputDir, pkg, child)
	}
	return nil
}
