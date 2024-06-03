module ergo.services/tools

go 1.21.6

require (
	ergo.services/application v0.0.0-20240603041555-11c33dbf0474
	ergo.services/ergo v1.999.225-0.20240603041303-2ed097599aea
	ergo.services/logger v0.0.0-20240221211214-98de4c9ff50e
	ergo.services/registrar v0.0.0-20240221075028-84be09c83208
	github.com/fsnotify/fsnotify v1.7.0
	github.com/knadh/koanf/parsers/yaml v0.1.0
	github.com/knadh/koanf/providers/file v0.1.0
	github.com/knadh/koanf/v2 v2.1.0
)

require (
	ergo.services/meta v0.0.0-20240221070545-d828c9b7f13e // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	golang.org/x/sys v0.14.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace ergo.services/application => ../application
// replace ergo.services/ergo => ../ergo
