package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// loadTemplates parses all embedded templates with shared helper functions.
func loadTemplates() (*template.Template, error) {
	funcMap := template.FuncMap{
		"loName":      strings.ToLower,
		"renderChild": renderChildTree,
	}
	tmpl := template.New("").Funcs(funcMap)
	return tmpl.ParseFS(templateFS, "templates/*.tmpl")
}


// renderChildTree renders a single child node as an ASCII tree line.
func renderChildTree(c ChildSpec, indent int) string {
	prefix := strings.Repeat("  ", indent)
	var sb strings.Builder
	if c.IsSup() == true {
		sb.WriteString("\n" + prefix + "└─ [sup] " + c.Sup + " (" + c.Type + ")")
		for _, child := range c.Children {
			sb.WriteString(renderChildTree(child, indent+1))
		}
	} else {
		kind := "actor"
		if c.Pool == true {
			kind = "pool"
		}
		sb.WriteString("\n" + prefix + "└─ [" + kind + "] " + c.Actor)
	}
	return sb.String()
}

// Generate orchestrates code generation for the entire project into outputDir.
func Generate(proj *Project, outputDir string) error {
	tmplSet, err := loadTemplates()
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	// generate cmd/ (node entry point)
	if err := genNode(tmplSet, outputDir, proj); err != nil {
		return fmt.Errorf("generating node: %w", err)
	}

	// generate apps/
	for i := range proj.Node.Apps {
		app := &proj.Node.Apps[i]
		if err := genApp(tmplSet, outputDir, proj.Node.Module, app); err != nil {
			return fmt.Errorf("generating app %s: %w", app.Name, err)
		}
	}

	// generate messages
	rootPkg := rootPackageName(proj.Node.Module)
	if err := genMessages(tmplSet, outputDir, rootPkg, proj.Node.Messages); err != nil {
		return fmt.Errorf("generating messages: %w", err)
	}

	// go mod init + go mod tidy
	if err := runGoMod(outputDir, proj.Node.Module); err != nil {
		return fmt.Errorf("go mod: %w", err)
	}

	return nil
}

// rootPackageName extracts the last path element of a module name for use as package name.
func rootPackageName(module string) string {
	parts := strings.Split(module, "/")
	name := parts[len(parts)-1]
	// sanitize: replace hyphens and dots
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, ".", "")
	return strings.ToLower(name)
}

// runGoMod runs go mod init (if needed) and go mod tidy in the given directory.
func runGoMod(dir string, module string) error {
	modFile := dir + "/go.mod"
	if _, err := os.Stat(modFile); os.IsNotExist(err) {
		cmd := exec.Command("go", "mod", "init", module)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go mod init: %w", err)
		}
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	return nil
}
