<a href="https://ergo.services"><img src="../.github/logo.green.svg" alt="Ergo Framework" width="298" height="49"></a>

This is the boilerplate code generator to create a service with Ergo Framework. To install it, use the following command:

  `go install ergo.services/tools/ergo@latest`

Please follow this pattern, keeping the order of declaration according to the supervision tree of your project:

  `ParentActor:Actor{param1:value1,param2:value2...}`

#### Options
  - `-init` Node name

    params:
    - `ssl:yes` enables SSL for the node
    - `module` defines module name

     example:
       `ergo -init "myService{ssl:yes,module:github.com/user/example}"`
  - `-path` defines location for the generated code
  - `-with-actor` add actor (based on `gen.Server`)
  - `-with-app` add application (based on `gen.Application`)
  - `-with-sup` add supervisor (based on `gen.Supervisor`)
  - `-with-cloud` enables Cloud feature for the node
  - `-with-msg` add message for the networking

    params:
	- `strict:yes` enable strict mode for unmarshaling message
  - `-with-pool` add pool of workers (based on `gen.Pool`)

    params:
	- `workers` number of starting workers
  - `-with-raft` add raft (based on `gen.Raft`)
  - `-with-saga` add saga (based on `gen.Saga`)
  - `-with-stage` add stage (based on `gen.Stage`)
  - `-with-tcp` add TCP server (based on `gen.TCP`)

    params:
    - `ssl:yes` enables SSL for this TCP server
    - `host` defines hostname
    - `port` defines port number
	- `handlers` number of starting handlers
  - `-with-udp` add UDP server (based on `gen.UDP`)

    params:
    - `host` defines hostname
    - `port` defines port number
	- `handlers` number of starting handlers
  - `-with-web` add Web server (based on `gen.Web`)

    params:
    - `ssl:yes` enables SSL for this Web server
    - `host` defines hostname
    - `port` defines port number
	- `handlers` number of starting handlers

   See `ergo -help` for more information

   ### Example:

   Supervision tree
   ```
   mynode
   |- myapp
   |   |
   |    `- mysup
   |        |
   |         `- myactor
   |- myweb
   `- myactor2
   ```

   To generate project for this design use the following command:

   `ergo -init MyNode -with-app MyApp -with-sup MyApp:MySup -with-actor MySup:MyActor -with-web "MyWeb{port:8000,handlers:3}" -with-actor MyActor2`

   as a result you will get generated project:

   ```
      mynode/
      |-- apps/
      |   `-- myapp/
      |       |-- myactor.go
      |       |-- myapp.go
      |       `-- mysup.go
      |-- cmd/
      |   |-- myactor2.go
      |   |-- mynode.go
      |   |-- myweb.go
      |   `-- myweb_handler.go
      |-- README.md
      |-- go.mod
      `-- go.sum
   ```

   to try it:
   ```
   $ cd mynode
   $ go run ./cmd/
   ```

