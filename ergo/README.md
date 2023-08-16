<a href="https://ergo.services"><img src="../.github/logo.green.svg" alt="Ergo Framework" width="298" height="49"></a>

This is the boilerplate code generator to create a service with Ergo Framework. To install it, use the following command:

  `go install ergo.services/tools/ergo@latest`

Please follow this pattern, keeping the order of declaration according to the supervision tree of your project:

  `ParentActor:Actor{param1:value1,param2:value2...}`

#### Options
  - `-init` Node name

    _params:_
    - `tls` enables TLS for the node (with generating self-signed certificate)
    - `module` defines module name

    _example:_
       `ergo -init "myService{tls,module:github.com/user/example}"`
  - `-path` defines location for the generated code
  - `-with-actor` add actor (based on `act.Actor`)
  - `-with-app` add application
  - `-with-sup` add supervisor (based on `act.Supervisor`)

    _params:_
    - `type` supervisor strategy type. available values:

        * `ofo` **(default)** one for one - If a child process terminates, only that process is restarted
        * `rfo` rest for one - If a child process terminates, the rest of the child processes are terminated, then the terminated child process and the rest of the child processes are restarted
        * `afo` all for one - If a child process terminates, all other child processes are terminated, and then all child processes, including the terminated one, are restarted
        * `sofo` simple one for one - is a simplified `ofo` supervisor, where all child processes are dynamically added instances of the same process

    - `strategy` restart strategy. available values:

        * `trans` **(default)** transient - child process is restarted only if it terminates abnormally
        * `perm` permanent - child process is always restarted
        * `temp` temporary - child process is never restarted

    _example:_
       `ergo -init myService -with-sup{type:rfo,strategy:perm}"`
  - `-with-cloud` add Cloud Client application to be connected to cloud https://ergo.services
  - `-with-msg` add message for the networking

    _params:_
	- `strict:yes` enable strict mode for unmarshaling message
  - `-with-pool` add pool of workers (based on `act.Pool`)

    _params:_
	- `size` number of workers in the pool
  - `-with-tcp` add TCP server (based on `act.TCP`)

    _params:_
    - `tls` enables node's certificate for this TCP server
    - `host` defines hostname
    - `port` defines port number
  - `-with-udp` add UDP server (based on `act.UDP`)

    _params:_
    - `host` defines hostname
    - `port` defines port number
  - `-with-web` add Web server (based on `act.Web`)

    _params:_
    - `tls` enables node's certificate for this Web server
    - `host` defines hostname
    - `port` defines port number
	- `websocket` add websocket handler (uses "/ws" endpoint)

   See https://docs.ergo.services/tools/ergo for more information

