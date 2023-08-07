<a href="https://ergo.services"><img src="../.github/logo.green.svg" alt="Ergo Framework" width="298" height="49"></a>

This is the boilerplate code generator to create a service with Ergo Framework. To install it, use the following command:

  `go install ergo.services/tools/ergo@latest`

Please follow this pattern, keeping the order of declaration according to the supervision tree of your project:

  `ParentActor:Actor{param1:value1,param2:value2...}`

#### Options
  - `-init` Node name

    _params:_
    - `ssl:yes` enables SSL for the node
    - `module` defines module name

    _example:_
       `ergo -init "myService{ssl:yes,module:github.com/user/example}"`
  - `-path` defines location for the generated code
  - `-with-actor` add actor (based on `gen.Server`)
  - `-with-app` add application (based on `gen.Application`)
  - `-with-sup` add supervisor (based on `gen.Supervisor`)

    _params:_
    - `type` supervisor strategy type. available values:

        * `ofo` **(default)** one for one - If a child process terminates, only that process is restarted
        * `rfo` rest for one - If a child process terminates, the rest of the child processes are terminated, then the terminated child process and the rest of the child processes are restarted
        * `ofa` one for all - If a child process terminates, all other child processes are terminated, and then all child processes, including the terminated one, are restarted
        * `sofo` simple one for one - is a simplified `ofo` supervisor, where all child processes are dynamically added instances of the same process

    - `restart` restart strategy. available values:

        * `trans` **(default)** transient - child process is restarted only if it terminates abnormally
        * `perm` permanent - child process is always restarted
        * `temp` temporary - child process is never restarted

    _example:_
       `ergo -init myService -with-sup{type:rfo,restart:perm}"`
  - `-with-cloud` enables Cloud feature for the node
  - `-with-msg` add message for the networking

    _params:_
	- `strict:yes` enable strict mode for unmarshaling message
  - `-with-pool` add pool of workers (based on `gen.Pool`)

    _params:_
	- `workers` number of starting workers
  - `-with-raft` add raft (based on `gen.Raft`)
  - `-with-saga` add saga (based on `gen.Saga`)
  - `-with-stage` add stage (based on `gen.Stage`)
  - `-with-tcp` add TCP server (based on `gen.TCP`)

    _params:_
    - `ssl:yes` enables SSL for this TCP server
    - `host` defines hostname
    - `port` defines port number
	- `handlers` number of starting handlers
  - `-with-udp` add UDP server (based on `gen.UDP`)

    _params:_
    - `host` defines hostname
    - `port` defines port number
	- `handlers` number of starting handlers
  - `-with-web` add Web server (based on `gen.Web`)

    _params:_
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

