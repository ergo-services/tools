// Package meta is a stand-in for the framework's meta package. Only the web request
// message matters here: A2008 reads its type identity and its Done field.
package meta

import "net/http"

type MessageWebRequest struct {
	Response http.ResponseWriter
	Request  *http.Request
	Done     func()
}

type WebHandlerOptions struct {
	Worker         string
	RequestTimeout int64
}
