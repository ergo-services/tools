# ergo

Boilerplate generator for [Ergo Framework](https://ergo.services) projects.

Scaffolds a working Go project with actors, supervisors and applications in seconds.
Supports incremental development: add components one by one as your project grows.

## Installation

```
go install ergo.tools/ergo@latest
```

## Quick start

```
ergo init MyNode github.com/myorg/mynode
cd mynode
go run ./cmd
```

That is all. You get a running Ergo node with an application, a supervisor and an actor.

## How it works

`ergo init` creates a project and an `ergo.yaml` file that describes its structure.
Use `ergo add` to grow the project incrementally. Each command updates `ergo.yaml`
and regenerates the affected files.

Every component follows the same pattern: a generated file handles wiring,
a user-owned file holds your logic:

| File | Owned by | Contains |
|------|----------|---------|
| `name_gen.go` | generator, always regenerated | factory, Init spec, Load group |
| `name.go` | you, never overwritten | Tune, handlers, Start, Terminate |

The wiring files stay in sync with `ergo.yaml` automatically. Your files are
written once and never touched again.

User-owned hooks follow a consistent pattern across all component types:

| File | Hook | Purpose |
|------|------|---------|
| `mysup.go` | `Tune(spec, args) (SupervisorSpec, error)` | adjust supervisor spec |
| `myapp.go` | `Tune(node, spec, args) (ApplicationSpec, error)` | adjust application spec |
| `messages.go` | `extraMessages() []any` | register custom EDF types |
| `cmd/main.go` | `extraApps() []gen.ApplicationBehavior` | add external applications |

## Commands

### ergo init

```
ergo init <NodeName> <module>
```

Creates a new project in a directory named after the last segment of the module path.
Generates `ergo.yaml`, all boilerplate files, `go.mod`, and runs `go mod tidy`.

```
ergo init MyNode github.com/myorg/mynode
ergo init Gateway github.com/myorg/gateway
```

### ergo add actor

```
ergo add actor [--pool] <[Parent:]Name>
```

Adds an actor. `Parent` is the name of an existing supervisor or application.
Without a parent the actor is added to `node.processes` and spawned directly by the node.

`--pool` generates a pool actor with a companion worker type. The pool distributes
incoming messages across a fixed set of workers and restarts them automatically.

```
ergo add actor MySup:MyActor
ergo add actor --pool MySup:MyPool
ergo add actor StandaloneActor
```

### ergo add supervisor

```
ergo add supervisor [--type <type>] [--strategy <strategy>] <[Parent:]Name>
```

Adds a supervisor. `Parent` is an existing application or supervisor.

`--type` controls which children are restarted on failure:
- `one_for_one` (default): only the failed child
- `all_for_one`: all children
- `rest_for_one`: the failed child and all children started after it
- `simple_one_for_one`: children spawned dynamically at runtime via `AddChild`

`--strategy` controls when a child is restarted:
- `transient` (default): only on abnormal exit
- `permanent`: always
- `temporary`: never

```
ergo add supervisor MyApp:MySup
ergo add supervisor MySup:SubSup --type all_for_one --strategy permanent
```

### ergo add app

```
ergo add app [--mode <mode>] <Name>
```

Adds an application. `--mode` controls what happens when the application stops:
- `transient` (default): node stops on abnormal exit
- `permanent`: node always stops
- `temporary`: node ignores the exit

```
ergo add app MyApp
ergo add app BackgroundApp --mode temporary
```

### ergo add message

```
ergo add message --field name:type [--field name:type ...] <Name>
```

Adds an EDF message type. Field types can be standard Go types (`string`, `int`,
`bool`, `[]byte`) or framework types (`gen.Alias`, `gen.PID`, `gen.Ref`).

Generated struct definitions and EDF registration go into `messages_gen.go`,
which is always regenerated. To register additional custom types, add them to
`extraMessages()` in the user-owned `messages.go`.

```
ergo add message MessageConnect --field ID:gen.Alias --field Addr:string
ergo add message MessageData --field ID:gen.Alias --field Payload:"[]byte"
```

### ergo generate

```
ergo generate [ergo.yaml]
```

Regenerates all `*_gen.go` files from `ergo.yaml`. Your `.go` files are never
overwritten. Searches for `ergo.yaml` in the current directory and its parents.

```
ergo generate
ergo generate /path/to/ergo.yaml
```

## Project structure

```
mynode/
  ergo.yaml               project definition (updated by ergo add)
  go.mod
  go.sum
  messages_gen.go         EDF structs + registration  (generated)
  messages.go             extraMessages() hook        (yours)
  apps/
    myapp/
      myapp_gen.go        CreateApp, Load             (generated)
      myapp.go            Tune, Start, Terminate      (yours)
      mysup_gen.go        factory, Init               (generated)
      mysup.go            Tune, HandleMessage         (yours)
      myactor_gen.go      factory                     (generated)
      myactor.go          Init, HandleMessage         (yours)
  cmd/
    main_gen.go           node startup, app list      (generated)
    main.go               extraApps() hook            (yours)
```

## ergo.yaml

```yaml
node:
  name: MyNode
  module: github.com/myorg/mynode
  host: localhost
  network:
    tls: false
    cookie: ""           # empty = auto-generated
  loggers:               # colored, rotate
    - colored
  apps:
    - name: MyApp
      mode: transient
      children:
        - sup: MySup
          type: one_for_one
          strategy: transient
          intensity: 2
          period: 5
          children:
            - actor: MyActor
            - actor: MyPool
              pool: true
    - observer             # known extras: observer, mcp, radar
  processes:
    - actor: StandaloneActor
  messages:
    - name: MessageConnect
      fields:
        - ID: gen.Alias
        - Addr: string
```

## Customizing generated code

### Supervisor spec

`mysup.go` contains `Tune`, called from the generated `Init`. The generated `Init`
builds the `SupervisorSpec` from `ergo.yaml`. `Tune` can modify it before startup:

```go
func (sup *MySup) Tune(spec act.SupervisorSpec, args ...any) (act.SupervisorSpec, error) {
    spec.Restart.Intensity = 10
    spec.Restart.Period = 30
    return spec, nil
}
```

### Application spec

`myapp.go` contains `Tune`, called from the generated `Load`. Use it to set
metadata, environment variables or dependencies:

```go
func (app *MyApp) Tune(node gen.Node, spec gen.ApplicationSpec, args ...any) (gen.ApplicationSpec, error) {
    spec.Description = "my application"
    spec.Version = gen.Version{Release: "1.0.0"}
    spec.Env = map[gen.Env]any{"DB_HOST": "localhost"}
    return spec, nil
}
```

### Custom EDF message types

`messages.go` contains `extraMessages()`, called from the generated `init()`.
Add custom types that are not declared in `ergo.yaml`:

```go
func extraMessages() []any {
    return []any{
        MyCustomMessage{},
        AnotherMessage{},
    }
}
```

### External applications

`cmd/main.go` contains `extraApps()`. Add third-party applications that need
custom constructor arguments not expressible in `ergo.yaml`:

```go
func extraApps() []gen.ApplicationBehavior {
    return []gen.ApplicationBehavior{
        thirdparty.New(thirdparty.Options{Port: 8080}),
    }
}
```

## Typical workflow

```bash
# 1. scaffold
ergo init MyNode github.com/myorg/mynode
cd mynode

# 2. run immediately
go run ./cmd

# 3. add components as needed
ergo add supervisor MyNodeApp:ApiSup --type one_for_one
ergo add actor ApiSup:HttpHandler
ergo add actor --pool ApiSup:RequestPool
ergo add message MessageRequest --field ID:gen.Alias --field Body:string

# 4. implement logic in the .go files
# 5. run again
go run ./cmd
```

## Documentation

https://docs.ergo.services/tools/ergo
