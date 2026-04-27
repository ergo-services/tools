package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"text/template"
)

// appData is passed to app templates.
type appData struct {
	Package     string
	Name        string
	LoName      string
	Mode        string
	TopChildren []ChildSpec
}

// genApp generates all files for a user-defined application and recurses into children.
func genApp(tmplSet *template.Template, outputDir string, module string, app *AppSpec) error {
	if app.Extra != "" {
		// known extras have no generated files
		return nil
	}

	pkg := strings.ToLower(app.Name)
	appDir := filepath.Join(outputDir, "apps", pkg)

	data := appData{
		Package:     pkg,
		Name:        app.Name,
		LoName:      strings.ToLower(app.Name),
		Mode:        appMode(app.Mode),
		TopChildren: app.Children,
	}

	// app_gen.go
	var genBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&genBuf, "app_gen.tmpl", data); err != nil {
		return err
	}
	genPath := filepath.Join(appDir, strings.ToLower(app.Name)+"_gen.go")
	if err := writeGenFile(genPath, genBuf.Bytes()); err != nil {
		return err
	}

	// app user file
	var userBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&userBuf, "app_user.tmpl", data); err != nil {
		return err
	}
	userPath := filepath.Join(appDir, strings.ToLower(app.Name)+".go")
	if err := writeUserFile(userPath, userBuf.Bytes()); err != nil {
		return err
	}

	// recurse into app's children
	for i := range app.Children {
		c := &app.Children[i]
		if err := genChild(tmplSet, appDir, pkg, c); err != nil {
			return err
		}
	}

	return nil
}
