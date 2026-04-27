package main

import (
	"bytes"
	"path/filepath"
	"text/template"
)

// fieldData represents a single message field for template rendering.
type fieldData struct {
	FieldName string
	FieldType string
}

// messageData is the data passed to message templates for a single message.
type messageData struct {
	Name   string
	Fields []fieldData
}

// messagesGenData is the data for messages_gen.tmpl.
type messagesGenData struct {
	Package  string
	Messages []messageData
}

// genMessages generates messages_gen.go (struct definitions + edf registration,
// always regenerated) and messages.go (user-owned extraMessages() hook, written once).
// messages.go is always created so that extraMessages() is always defined,
// even when no messages are declared in ergo.yaml yet.
func genMessages(tmplSet *template.Template, outputDir string, pkg string, messages []MessageSpec) error {
	msgs := make([]messageData, 0, len(messages))
	for _, m := range messages {
		var fields []fieldData
		for _, f := range m.Fields {
			for k, v := range f {
				fields = append(fields, fieldData{FieldName: k, FieldType: v})
			}
		}
		msgs = append(msgs, messageData{Name: m.Name, Fields: fields})
	}

	genData := messagesGenData{Package: pkg, Messages: msgs}

	// messages_gen.go — always regenerated when messages exist
	if len(messages) > 0 {
		var genBuf bytes.Buffer
		if err := tmplSet.ExecuteTemplate(&genBuf, "messages_gen.tmpl", genData); err != nil {
			return err
		}
		if err := writeGenFile(filepath.Join(outputDir, "messages_gen.go"), genBuf.Bytes()); err != nil {
			return err
		}
	}

	// messages.go — user-owned extraMessages() hook, written only once
	var userBuf bytes.Buffer
	if err := tmplSet.ExecuteTemplate(&userBuf, "messages_user.tmpl", genData); err != nil {
		return err
	}
	return writeUserFile(filepath.Join(outputDir, "messages.go"), userBuf.Bytes())
}
