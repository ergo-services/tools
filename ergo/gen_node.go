package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// loggerImport holds import data for a logger.
type loggerImport struct {
	Alias  string
	Import string
}

// extraAppImport holds import/create data for a known extra app.
type extraAppImport struct {
	Alias       string
	Import      string
	Create      string
	DefaultArgs string
}

// mainGenData is the data passed to main_gen.tmpl.
type mainGenData struct {
	Module        string
	NodeName      string
	Host          string
	TLS           bool
	Loggers       []loggerImport
	LoggerImports []loggerImport
	UserApps      []AppSpec
	ExtraApps     []extraAppImport
	Processes     []ChildSpec
}

// genNode generates the cmd/ directory files.
func genNode(tmplSet *template.Template, outputDir string, proj *Project) error {
	cmdDir := filepath.Join(outputDir, "cmd")

	node := proj.Node
	host := node.Host
	if host == "" {
		host = "localhost"
	}

	// split user apps and extra apps
	var userApps []AppSpec
	var extraApps []extraAppImport

	for _, app := range node.Apps {
		if app.Extra != "" {
			info, ok := knownExtras[app.Extra]
			if ok == false {
				// unknown extra, skip
				continue
			}
			alias := app.Extra
			extraApps = append(extraApps, extraAppImport{
				Alias:       alias,
				Import:      info.Import,
				Create:      info.Create,
				DefaultArgs: info.Args,
			})
		} else {
			userApps = append(userApps, app)
		}
	}

	// build logger imports
	var loggers []loggerImport
	var loggerImports []loggerImport
	for _, l := range node.Loggers {
		imp, ok := knownLoggers[l]
		if ok == false {
			continue
		}
		li := loggerImport{Alias: l, Import: imp}
		loggers = append(loggers, li)
		loggerImports = append(loggerImports, li)
	}

	data := mainGenData{
		Module:        node.Module,
		NodeName:      node.Name,
		Host:          host,
		TLS:           node.Network.TLS,
		Loggers:       loggers,
		LoggerImports: loggerImports,
		UserApps:      userApps,
		ExtraApps:     extraApps,
		Processes:     node.Processes,
	}

	// main_gen.go
	var genBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&genBuf, "main_gen.tmpl", data); err != nil {
		return err
	}
	if err := writeGenFile(filepath.Join(cmdDir, "main_gen.go"), genBuf.Bytes()); err != nil {
		return err
	}

	// main.go (user-owned)
	var userBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&userBuf, "main_user.tmpl", nil); err != nil {
		return err
	}
	if err := writeUserFile(filepath.Join(cmdDir, "main.go"), userBuf.Bytes()); err != nil {
		return err
	}

	// generate files for node.processes actors/sups in cmd/
	for i := range node.Processes {
		c := &node.Processes[i]
		if err := genChild(tmplSet, cmdDir, "main", c); err != nil {
			return err
		}
	}

	// generate README.md (always overwrite - it's documentation, not user logic)
	type readmeApp struct {
		Name     string
		Mode     string
		IsExtra  bool
		Children []ChildSpec
	}
	var readmeApps []readmeApp
	for _, app := range node.Apps {
		if app.Extra != "" {
			readmeApps = append(readmeApps, readmeApp{Name: app.Extra, IsExtra: true})
		} else {
			readmeApps = append(readmeApps, readmeApp{
				Name:     app.Name,
				Mode:     app.Mode,
				Children: app.Children,
			})
		}
	}
	readmeData := struct {
		NodeName  string
		Apps      []readmeApp
		Processes []ChildSpec
	}{
		NodeName:  node.Name,
		Apps:      readmeApps,
		Processes: node.Processes,
	}
	var readmeBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&readmeBuf, "readme.tmpl", readmeData); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"), readmeBuf.Bytes(), 0644); err != nil {
		return err
	}

	_ = strings.ToLower // suppress unused import
	return nil
}
