package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// readProject reads and parses an ergo.yaml file.
func readProject(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var proj Project
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &proj, nil
}

// writeProject serializes a Project to ergo.yaml.
func writeProject(path string, proj *Project) error {
	data, err := yaml.Marshal(proj)
	if err != nil {
		return fmt.Errorf("marshaling project: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// findSupChildren searches recursively through app children for a supervisor
// with the given name and returns a pointer to its Children slice.
func findSupChildren(children []ChildSpec, name string) *[]ChildSpec {
	for i := range children {
		c := &children[i]
		if c.IsSup() == true && c.Sup == name {
			return &c.Children
		}
		if c.IsSup() == true {
			if found := findSupChildren(c.Children, name); found != nil {
				return found
			}
		}
	}
	return nil
}

// findParentChildren returns a pointer to the children slice of the named
// parent (supervisor or app). It searches apps and their children.
func findParentChildren(proj *Project, parent string) (*[]ChildSpec, error) {
	// check each user app's top-level children
	for i := range proj.Node.Apps {
		app := &proj.Node.Apps[i]
		if app.Extra != "" {
			continue
		}
		if app.Name == parent {
			return &app.Children, nil
		}
		// search within app children
		if found := findSupChildren(app.Children, parent); found != nil {
			return found, nil
		}
	}
	// also search in node.processes supervisor children (not typical, but defensive)
	if found := findSupChildren(proj.Node.Processes, parent); found != nil {
		return found, nil
	}
	return nil, fmt.Errorf("parent %q not found in project tree", parent)
}

// childExists returns true if a child with the given name already exists in the slice.
func childExists(children []ChildSpec, name string) bool {
	for _, c := range children {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// addActorToProject adds an actor (or pool) child to the project tree.
// parentName may be empty (adds to node.processes) or a sup/app name.
func addActorToProject(proj *Project, parentName string, childName string, isPool bool) error {
	child := ChildSpec{Actor: childName, Pool: isPool}

	if parentName == "" {
		if childExists(proj.Node.Processes, childName) {
			return fmt.Errorf("actor %q already exists in processes", childName)
		}
		proj.Node.Processes = append(proj.Node.Processes, child)
		return nil
	}

	target, err := findParentChildren(proj, parentName)
	if err != nil {
		return err
	}
	if childExists(*target, childName) {
		return fmt.Errorf("actor %q already exists under %q", childName, parentName)
	}
	*target = append(*target, child)
	return nil
}

// addSupToProject adds a supervisor child to the project tree.
func addSupToProject(proj *Project, parentName string, spec ChildSpec) error {
	if parentName == "" {
		if childExists(proj.Node.Processes, spec.Sup) {
			return fmt.Errorf("supervisor %q already exists in processes", spec.Sup)
		}
		proj.Node.Processes = append(proj.Node.Processes, spec)
		return nil
	}

	target, err := findParentChildren(proj, parentName)
	if err != nil {
		return err
	}
	if childExists(*target, spec.Sup) {
		return fmt.Errorf("supervisor %q already exists under %q", spec.Sup, parentName)
	}
	*target = append(*target, spec)
	return nil
}

// addAppToProject adds a new user app to the project.
func addAppToProject(proj *Project, spec AppSpec) error {
	for _, a := range proj.Node.Apps {
		if a.Name == spec.Name {
			return fmt.Errorf("app %q already exists", spec.Name)
		}
	}
	proj.Node.Apps = append(proj.Node.Apps, spec)
	return nil
}

// addMessageToProject adds a message spec to the project.
func addMessageToProject(proj *Project, spec MessageSpec) error {
	for _, m := range proj.Node.Messages {
		if m.Name == spec.Name {
			return fmt.Errorf("message %q already exists", spec.Name)
		}
	}
	proj.Node.Messages = append(proj.Node.Messages, spec)
	return nil
}
