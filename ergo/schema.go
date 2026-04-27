package main

import "gopkg.in/yaml.v3"

// Project is the top-level structure for ergo.yaml.
type Project struct {
	Node NodeSpec `yaml:"node"`
}

// NodeSpec describes the node configuration.
type NodeSpec struct {
	Name      string        `yaml:"name"`
	Module    string        `yaml:"module"`
	Host      string        `yaml:"host"`
	Network   NetworkSpec   `yaml:"network"`
	Loggers   []string      `yaml:"loggers"`
	Apps      []AppSpec     `yaml:"apps"`
	Processes []ChildSpec   `yaml:"processes"`
	Messages  []MessageSpec `yaml:"messages"`
}

// NetworkSpec holds network-level settings.
type NetworkSpec struct {
	TLS    bool   `yaml:"tls"`
	Cookie string `yaml:"cookie"`
}

// AppSpec represents either a user-defined application (Name set)
// or a known extra application (Extra set, e.g. "observer").
type AppSpec struct {
	Name     string      `yaml:"name,omitempty"`
	Mode     string      `yaml:"mode,omitempty"`
	Children []ChildSpec `yaml:"children,omitempty"`
	Extra    string      `yaml:"-"` // set when unmarshaled from scalar string
}

// UnmarshalYAML handles both scalar strings (extras) and mappings (user apps).
func (a *AppSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		a.Extra = value.Value
		return nil
	}
	type appAlias AppSpec
	return value.Decode((*appAlias)(a))
}

// MarshalYAML emits a scalar for extras, otherwise the full struct.
func (a AppSpec) MarshalYAML() (any, error) {
	if a.Extra != "" {
		return a.Extra, nil
	}
	type appAlias AppSpec
	return appAlias(a), nil
}

// ChildSpec represents either a supervisor or an actor child.
type ChildSpec struct {
	// supervisor fields
	Sup       string      `yaml:"sup,omitempty"`
	Type      string      `yaml:"type,omitempty"`
	Strategy  string      `yaml:"strategy,omitempty"`
	Intensity int         `yaml:"intensity,omitempty"`
	Period    int         `yaml:"period,omitempty"`
	Children  []ChildSpec `yaml:"children,omitempty"`

	// actor fields
	Actor string `yaml:"actor,omitempty"`
	Pool  bool   `yaml:"pool,omitempty"`
}

// IsSup reports whether this child is a supervisor.
func (c *ChildSpec) IsSup() bool { return c.Sup != "" }

// IsActor reports whether this child is an actor.
func (c *ChildSpec) IsActor() bool { return c.Actor != "" }

// Name returns the name of the child regardless of kind.
func (c *ChildSpec) Name() string {
	if c.Sup != "" {
		return c.Sup
	}
	return c.Actor
}

// MessageSpec describes a message type with its fields.
type MessageSpec struct {
	Name   string               `yaml:"name"`
	Fields []map[string]string  `yaml:"fields"`
}
