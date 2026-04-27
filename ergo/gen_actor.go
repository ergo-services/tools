package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"text/template"
)

// genActor generates the _gen.go and user-owned .go files for an actor or pool.
func genActor(tmplSet *template.Template, outputDir string, pkg string, child *ChildSpec) error {
	name := child.Actor
	lname := strings.ToLower(name)
	isPool := child.Pool

	data := struct {
		Package string
		Name    string
		IsPool  bool
	}{
		Package: pkg,
		Name:    name,
		IsPool:  isPool,
	}

	// _gen.go file
	var genBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&genBuf, "actor_gen.tmpl", data); err != nil {
		return err
	}
	genPath := filepath.Join(outputDir, lname+"_gen.go")
	if err := writeGenFile(genPath, genBuf.Bytes()); err != nil {
		return err
	}

	// user-owned .go file
	var userBuf bytes.Buffer
	if isPool == true {
		if err := tmplSet.ExecuteTemplate(&userBuf, "pool_user.tmpl", data); err != nil {
			return err
		}
	} else {
		if err := tmplSet.ExecuteTemplate(&userBuf, "actor_user.tmpl", data); err != nil {
			return err
		}
	}
	userPath := filepath.Join(outputDir, lname+".go")
	return writeUserFile(userPath, userBuf.Bytes())
}
