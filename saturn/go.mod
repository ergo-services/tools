module ergo.services/tools

replace ergo.services/ergo => ../ergo

replace ergo.services/registrar/saturn => ../registrar/saturn

replace ergo.services/application/observer => ../application/observer

replace ergo.services/meta/websocket => ../meta/websocket

replace ergo.services/logger/colored => ../logger/colored

replace ergo.services/logger/rotate => ../logger/rotate

go 1.20

require (
	ergo.services/application/observer v0.0.0-00010101000000-000000000000
	ergo.services/ergo v0.0.0-00010101000000-000000000000
	ergo.services/logger/colored v0.0.0-00010101000000-000000000000
	ergo.services/registrar/saturn v0.0.0-00010101000000-000000000000
	github.com/fsnotify/fsnotify v1.6.0
	github.com/knadh/koanf/parsers/yaml v0.1.0
	github.com/knadh/koanf/providers/file v0.1.0
	github.com/knadh/koanf/v2 v2.0.1
)

require (
	ergo.services/meta/websocket v0.0.0-00010101000000-000000000000 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	golang.org/x/sys v0.10.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
