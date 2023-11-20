module ergo.services/tools

replace ergo.services/ergo => /home/taras/devel/ergo.services/ergo3

replace ergo.services/registrar/saturn => /home/taras/devel/ergo.services/registrar/saturn

//github.com/ergo-services/ergo3 v0.0.0-20231025111759-115fe4227255

replace ergo.services/application/observer => github.com/ergo-services/application/observer v0.0.0-20231019185923-7272d747bb77

replace ergo.services/meta/websocket => github.com/ergo-services/meta/websocket v0.0.0-20231025193935-1a251d98d2ce

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

replace ergo.services/logger/colored => github.com/ergo-services/logger/colored v0.0.0-20231019185545-a4ca01f1fb7f

replace ergo.services/logger/rotate => github.com/ergo-services/logger/rotate v0.0.0-20231019185545-a4ca01f1fb7f
