<h1><a href="https://ergo.services"><img src=".github/logo.green.svg" alt="Ergo tools" width="298" height="49"></a></h1>

[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-ergo__services-229ed9?style=flat&logo=telegram&logoColor=white)](https://t.me/ergo_services)
[![Reddit](https://img.shields.io/badge/Reddit-r/ergo__services-ff4500?style=plastic&logo=reddit&logoColor=white&style=flat)](https://reddit.com/r/ergo_services)


Tools that make your life easier working with Ergo Framework [https://github.com/ergo-services/ergo](https://github.com/ergo-services/ergo).

## ergo

  This is the boilerplate code generator to create a service with Ergo Framework. To install it, use the following command:

  `go install ergo.tools/ergo@latest`

  Doc: https://docs.ergo.services/tools/ergo

## argus

  This is a static analyser for the invariants of the Ergo actor model that the Go type system cannot express: a callback that stops its mailbox, a spec the framework rejects at init, a message type registration will not accept, an error that loses its identity on the way to another node. It runs as a vet tool, so it uses the build graph, the build tags and the per-package caching the go command already has.

  `go install ergo.tools/argus@latest`

  `go vet -vettool=$(which argus) ./...`

  Doc: https://docs.ergo.services/tools/argus

## saturn

  This is a central registrar for the nodes made with Ergo Framework. It provides a simple way
  - to discover other nodes
  - to discover applications running on the nodes
  - to propagate configuration on the fly (pushing updates) to the registered nodes.

  `go install ergo.tools/saturn@latest`

  Doc: https://docs.ergo.services/tools/saturn

