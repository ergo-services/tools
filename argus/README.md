# argus

A static analyser for the invariants of the Ergo actor model that the Go type system
cannot express.

The framework enforces most of its contract at run time: a callback that blocks stops
a mailbox, a registration with the wrong shape refuses to start a node, an error that
was never registered loses its identity on the way to another node. All of those are
correct programs as far as the compiler is concerned, and all of them fail somewhere
far from the edit that caused them. argus turns them into a diagnostic on the line
that has to change.

It is a `go/analysis` tool, so it runs as a vet tool with the build graph, build tags
and per-package caching the go command already has. It does not import the framework:
it resolves the surface through `go/types` by package path, which keeps it dependency
free and lets it analyse a project pinned to any framework version.

---

## Install

```bash
go install ergo.tools/argus@latest
```

## Run

Production mode. The go command drives it, one process per package:

```bash
go vet -vettool=$(which argus) ./...
```

Directly over package patterns, which is faster during development:

```bash
argus ./...
argus ./apps/orders/...
```

Over a module you are not standing in. Patterns resolve against the main module of the
working directory, exactly as with the go command, so the tool moves there first:

```bash
argus -C ../../application/observer ./...
```

Subcommands:

```bash
argus help              list the rules in this build
argus help A2020        the full documentation for one rule
argus baseline          read a keyed run on stdin, write a baseline file
```

## Output

```
apps/orders/router.go:276:11: [tier2] [A2020] this error is sent as a message and it is
built with fmt.Errorf(%w), whose operand lives in an unexported field EDF cannot reach:
the peer decodes the text and errors.Is stops matching the sentinel on the other side of
the hop. Use gen.Errorf, which carries the causes in an exported field
```

Every diagnostic names the tier, the rule, what the runtime will actually do, and the
fix. Tier 1 findings go through the analysis framework and fail the build; tier 2 goes
to stderr and does not, which is what keeps `go vet` usable while a project is still
adopting the tool.

---

## What it checks

Three tiers. Tier 1 is a defect in the actor model itself: the code is wrong on a
single node, today. Tier 2 is a contract the framework enforces at run time, usually
across a node boundary or a lifecycle edge. Tier 3 is hygiene and is off by default.

### Tier 1 - the actor model

| Rule | What it reports | Shape |
| --- | --- | --- |
| **A1001** | A payload sharing mutable memory with its sender. Local delivery does not copy, so the receiver gets a reference to memory the sender still owns. Narrowed by guardedness: a pointee that synchronizes itself is not reported. | `p.Send(pid, Msg{Items: a.items})` |
| **A1002** | An unbounded wait in a callback. `time.Sleep`, a naked `Lock`/`RLock`/`WaitGroup.Wait`, a channel operation outside a `select` with an escape hatch. Transitive through helpers. A framework request is bounded and is not reported. | `func (a *A) HandleMessage(...) { <-ch }` |
| **A1003** | A request addressed to the caller's own registered name, `ProcessID` or alias. It lands in the caller's own mailbox while the caller waits for the reply, so it burns the full request timeout and returns `ErrTimeout`. | `a.Call(a.Name(), req)` |
| **A1004** | A goroutine started in a callback that touches actor state. Actor fields are unsynchronized by design, and the process handle is actor-goroutine only. | `go func(){ a.n++ }()` |
| **A1005** | A meta field mutated from both of a meta's two goroutines. Reported on a mutating access on each side, including a method call through a reference, which is the form no assignment-based model would see. | `Start(){ w.Write(b) }` + `HandleMessage(){ w.Flush() }` |
| **A1006** | A send handing over a field of the sending actor's own state. Same hazard as A1001, different fix: the sender keeps mutating it, so copying at the send site papers over the sharing. | `a.Send(pid, Msg{Cache: a.cache})` |
| **A1007** | A method returning memory derived from its receiver, handed to a send or a spawn. A reslice of an internal buffer read by another actor without the owner's lock. | `a.Send(pid, buf.Bytes())` |
| **A1008** | `HandleCall` returning an error in the termination reason slot. It kills the process and sends no reply, so any peer with an unrecognized request can terminate the callee. | `return nil, gen.ErrUnsupported` |
| **A1010** | A goroutine with no recover boundary reachable from a callback. A panic there takes the node down instead of one process, and supervision never gets a say. | `go doWork()` |
| **A1011** | A blocking request in `Init` against the init budget. Both default to five seconds, so the spawner kills the process and returns `ErrTimeout` while `Init` keeps running. The verdict travels on the factory object. | `Init(){ a.Call(x, r) }` |
| **A1012** | `Node().Send` or `Node().Call` from inside a callback. The node routes on its own behalf, so the actor never enters `WaitResponse` and reports itself as Running for the whole wait. | `a.Node().Call(to, req)` |

### Tier 2 - the framework contract

| Rule | What it reports | Shape |
| --- | --- | --- |
| **A2001** | A state gated call where the state forbids it: `Terminate` runs with the state already Terminated, a meta `Init` runs before Start. Also a request to the caller's own PID, which the runtime refuses before routing. | `Terminate(){ a.Call(...) }` |
| **A2001a** | A discarded result hiding a certain failure: an error from a gated call in `Terminate`, or the `CancelFunc` of a periodic timer that then cannot be stopped. | `_, _ = a.SendEvery(pid, m, d)` |
| **A2002** | A `HandleCall` that can never reply: every path returns `(nil, nil)`, `ref` is never used, and no method of the behavior replies. | `HandleCall(...) { return nil, nil }` |
| **A2003** | Two replies for one request. An explicit `SendResponse` plus a non-nil result makes the runtime send a second response, which fills the caller's response channel and then costs a legitimate reply. | `a.SendResponse(f, r, x); return y, nil` |
| **A2004** | A registered type whose shape registration refuses: an unexported field, `**T`, a chan or func, an interface other than `any` or `error`, a cycle. | `type M struct{ conn net.Conn }` |
| **A2005** | A supervisor spec init rejects: empty `Children`, empty `Name`, nil `Factory`, per-child `Intensity` under All/RestForOne, `Period` without `Intensity`, duplicate names. | `spec.Children = nil` |
| **A2006** | Timer misuse: a non-positive period, which never arms the ticker, and a raw `time.NewTicker`/`Tick`/`After` scheduling work off the mailbox. | `a.SendEvery(pid, m, 0)` |
| **A2007** | An event that can never be published: the registration token was discarded, the publish passes a zero `Ref`, or one name is registered twice. Subscribers link successfully and receive nothing. | `a.RegisterEvent("x", opts)` as a statement |
| **A2008** | A web request never completed with `Done`, so the HTTP goroutine stays parked and the client gets a 504; or the raw request handled on an `act.WebWorker`, where the run loop already consumed it. | `case meta.MessageWebRequest:` without `Done` |
| **A2009** | A registration that fails at node start: a pointer, a concrete error type, a bare builtin or a framework value type, a half implemented marshaler pair, a value receiver `UnmarshalEDF`, a registration placed after a peer may have connected, a deprecated package level `edf` entry point, an application member `InitTimeout` above the ceiling. | `Network().RegisterType(&M{})` |
| **A2011** | A spawn argument sharing unsynchronized memory with the parent, or a func argument that pins the child to this node. | `a.Spawn(f, opts, repo)` |
| **A2012** | A transient failure logged at Error and then returned as a termination reason, which restarts the process and regenerates the same condition. | `a.Log().Error(...); return err` |
| **A2013** | The event buffer returned by `LinkEvent` or `MonitorEvent` discarded, where the producer's buffer is known to be non-empty. | `a.LinkEvent(ev)` as a statement |
| **A2014** | A round trip in `Terminate`. `node.Wait` has already returned and the supervisor has already been told this process is down, so the write is abandoned or lands after the next incarnation's. | `Terminate(){ db.Exec(...) }` |
| **A2014a** | `Terminate` dereferencing a field `Init` assigns only after its first fallible step. `Terminate` also runs when `Init` fails, against exactly that half built receiver. | `Terminate(){ a.conn.Close() }` |
| **A2015** | A self timer chain armed from a second place, so two self-sustaining chains run in parallel and every arrival adds another. | two `a.SendAfter(a.PID(), tick{}, d)` |
| **A2016** | `RegisterName`, `RegisterEvent` or `CreateAlias` performed in a message handler that `Init` deferred to. `Spawn` has already returned the process to its parent, so a peer addressing it gets `ErrProcessUnknown` and nothing retries. | `Init(){ a.Send(a.PID(), setup{}) }` |
| **A2017** | `Init` returning a framework sentinel as control flow. Init's error is the spawn failure channel, so every caller has to know that this particular error means success. | `return gen.TerminateReasonNormal` |
| **A2018** | `EventOptions.Notify` set while the producer handles neither `MessageEventStart` nor `MessageEventStop`, so it cannot tell whether anyone is listening. | `EventOptions{Notify: true}` |
| **A2020** | An `fmt.Errorf` value with `%w` reaching another process. The operand lives in an unexported field EDF cannot reach, so the peer decodes the text and `errors.Is` stops matching while the message still reads correctly. `gen.Errorf` is the drop-in fix. | `Response{Error: fmt.Errorf("x: %w", ErrY)}` |
| **A2021** | A named type reachable through a registered type's fields that nobody registers. Registration resolves the field graph eagerly and answers "no encoder for type X", so the node that does not get it from another list refuses to start. | `RegisterTypes([]any{Order{}})` where `Order.Status` is unregistered |
| **A2022** | A package level sentinel put on the wire but missing from this package's error registration list. It is encoded as its text, so `errors.Is` answers false after the hop. | `Response{Error: ErrNotFound}` |
| **A2023** | An application `Init` returning an error after acquiring a resource. An application whose `Init` fails never gets its `Terminate` called, so nothing closes what it opened. | `a.pool = p; ...; return err` |
| **A2024** | A round trip inside an actor callback. An actor handles one message at a time, so everything queued behind it waits for the answer too. The surface is configured; see below. | `HandleMessage(){ repo.Load(ctx) }` |
| **A2025** | Router or pool options init rejects: an empty route name, a nil route factory, a duplicate route name, a missing `WorkerFactory`. | `act.RouterOptions{Routes: [...]}` |
| **A2026** | A call the runtime rejects on its arguments, whatever the state: an exit signal to self, parent or leader, a nil termination reason, a compression threshold below the floor, an enum value out of range, a self link or monitor, an address whose type is none of `gen.PID`, `gen.ProcessID`, `gen.Alias` or `gen.Atom`. | `a.Send("worker", msg)` |
| **A2027** | A meta `Start` that returns without blocking. The meta ends when `Start` returns, so the alias `SpawnMeta` just handed back is already dead. | `Start() error { return nil }` |
| **A2028** | A request inside `HandleCall` whose budget is not smaller than the caller's. The caller is waiting on its own default five seconds, so the inner wait can outlast the outer one and the reply lands after the caller gave up. An actor also handles one message at a time, so a pool of workers in front of one such handler serialises and every request past the first blows its budget queueing. | `HandleCall(...) { return a.Call(peer, req) }` |

### Tier 3 - hygiene, off by default

| Rule | What it reports |
| --- | --- |
| **A3001** | A type used as a message with no `//argus:message` marker, so it is checked only where it is sent rather than once at its declaration. |
| **A3003** | A type used as a message here but missing from this package's registration list. Silent for a package that registers nothing. |
| **A3005** | Suppression debt: a directive with no reason, a placeholder reason, a directive or baseline entry naming an unknown rule, and the per-package count of suppressed findings with the effective severity of each. |
| **A3006** | A doc comment already saying a type is local, which is what `//argus:message local` records. |
| **A3007** | A `Compression.Threshold` below the framework floor in a spawn literal, which is applied verbatim. |

The numbering has gaps. `A1009`, `A2010`, `A2019`, `A3002` and `A3004` are not in this
build, so a run that goes straight from `A2018` to `A2020` is complete rather than
broken. A rule identifier is permanent once published, because it appears in
configuration files, baselines and `//argus:allow` directives spread across a
codebase, so a number is never reused for a different rule. Do not fill a gap with a
rule of your own.

---

## Configuration

argus runs on sane defaults with no configuration at all. An `argus.yml` is discovered
by walking up from the analysed file and stopping at the enclosing `go.mod`, so a
dependency's config is never adopted. Point at one explicitly with
`-argusmodel.config=/path/to/argus.yml`.

```yaml
version: 1

# Severity per tier. off, warn or error.
# Default: tier1 error, tier2 warn, tier3 off.
# Only error fails the build; warn is written to stderr.
tiers:
  tier1: error
  tier2: warn
  tier3: off

# Accepted findings, relative to this file. See Adoption below.
baseline: argus-baseline.json

# Analyse test variants. Default true.
tests: true

allow:
  # Types that are safe to share by their own contract. A configured list
  # REPLACES the built-in one, so name the entries you still want.
  types:
    - { path: "[]byte", reason: payload, ownership transferred by convention }
    - { path: github.com/shopspring/decimal.Decimal, reason: immutable by contract }
  # Package subtrees excused wholesale. "/..." matches the subtree.
  packages:
    - { path: "example.com/app/generated/...", reason: generated code }

interfaces:
  # Interface types EDF has a built-in encoder for.
  accept: [error]

surfaces:
  # Calls that wait with no bound. transitive:false means a caller does not
  # inherit the verdict, which is right for a mutex and wrong for a sleep.
  blocking:
    - { func: time.Sleep, why: sleep }
    - { recv: sync.Mutex, method: Lock, why: mutex, transitive: false }

  # Calls whose cost is a round trip to something outside this process.
  # This is the list A2024 and A2014 read. Naming your own repository
  # interface here is what makes A2024 useful: an interface method's
  # receiver resolves exactly like a concrete one.
  roundtrip:
    - { recv: "example.com/app/ports.OrderRepo", method: Load, why: a database round trip }
    - { recv: "example.com/app/ports.OrderRepo", method: Save, why: a database round trip }
    - { recv: net/http.Client, method: Do, why: an HTTP round trip }

  # Methods taking a message payload, with the argument index.
  senders:
    - { recv: "example.com/app/bus.Bus", method: Publish, param: 1 }

  # Interfaces whose implementations are behaviors, and the callback names.
  callbacks:
    - recv: ergo.services/ergo/gen.ProcessBehavior
      methods: [Init, HandleMessage, HandleCall, Terminate]
```

A malformed config is a hard error rather than a silent fallback: failing open on a bad
surface block would switch off the rules that depend on it.

---

## Directives

A recorded exception sits on the reported line or the line above it.

```go
//argus:allow A1001 the receiver copies before it stores
p.Send(pid, Msg{Items: a.items})

//argus:allow A1001,A2011 both are the same deliberate sharing
p.Spawn(factory, opts, a.cache)

//argus:ignore this whole line is generated
p.Send(pid, generated)
```

`argus:allow` takes a comma separated rule list and a reason. `argus:ignore` waives every
rule on the line. Both are counted by A3005, and a directive with no reason, or with the
`<reason>` placeholder the offered quick fix emits, is reported.

Two markers move a type into definition-site checking:

```go
//argus:message
type Order struct{ ... }        // checked at its declaration, not only where it is sent

//argus:message local
type cursor struct{ ... }       // never crosses the wire, so A2004 and A3003 skip it
```

---

## Adoption on an existing codebase

The first run on real code reports findings that are all true and none of which are
getting fixed this week. Record them and start from zero:

```bash
argus -argusmodel.keys ./... 2>&1 | argus baseline > argus-baseline.json
```

Then point `argus.yml` at the file. An entry is keyed on the rule, the kind of finding
and the declaration it belongs to, never on a line number, so an edit above a finding
keeps its entry. Each entry carries a count meaning "accept the first N occurrences of
this identity", so the same defect appearing at a new site still reports.

An entry also records a witness. When the finding still matches by identity but now
describes something else, A3005 reports the entry as stale rather than accepting a
different defect under an old decision.

The baseline is a normal reviewable JSON file. Deleting an entry is how a fix is
recorded.

---

## CI

### GitHub Actions

```yaml
name: argus

on:
  push:
    branches: [main]
  pull_request:

jobs:
  argus:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Install argus
        run: go install ergo.tools/argus@latest

      - name: Vet
        run: go vet -vettool=$(go env GOPATH)/bin/argus ./...
```

`go vet` exits non-zero on a tier 1 finding, and on tier 2 or tier 3 only if the project
raised those tiers to `error` in `argus.yml`. Tier 2 findings are written to stderr and
appear in the job log either way.

To make the whole suite blocking, raise the tiers in `argus.yml` rather than changing the
workflow, so a local run and CI agree:

```yaml
tiers:
  tier1: error
  tier2: error
  tier3: off
```

To surface findings as annotations on the diff instead of as a failed step, run argus
directly and convert its output:

```yaml
      - name: Argus
        run: |
          argus ./... 2>&1 | tee argus.log
          awk -F: '/\[tier/ {
            file=$1; line=$2; col=$3;
            sub(/^[^:]*:[^:]*:[^:]*: /, "", $0);
            printf "::warning file=%s,line=%s,col=%s::%s\n", file, line, col, $0
          }' argus.log
```

### Keeping the baseline honest

A baseline that is never regenerated silently accumulates. This job fails when the
baseline accepts findings that no longer exist, which is what makes deleting entries part
of the fix rather than an afterthought:

```yaml
      - name: Baseline drift
        run: |
          argus -argusmodel.keys ./... 2>&1 | argus baseline > argus-baseline.new
          diff -u argus-baseline.json argus-baseline.new
```

### Other CI

Nothing about the tool is GitHub specific. The two commands any pipeline needs are:

```bash
go vet -vettool=$(go env GOPATH)/bin/argus ./...   # blocking, cached, uses the build graph
argus ./...                                        # everything, including the warn tier
```

A repository whose modules are finer than the repo runs one invocation per module:

```bash
for mod in $(find . -name go.mod -not -path '*/vendor/*'); do
  argus -C "$(dirname "$mod")" ./... || exit 1
done
```

---

## How it works

One analyzer builds the model in a single traversal per package and every rule reads it:
callbacks and which behavior family they belong to, send and spawn sites, message shapes
over two axes (does this share memory, would registration accept it), guardedness of the
types behind a reference, resolved supervisor, router and pool options with per-field
provenance, event registrations and subscriptions, factories and the init budget they
carry, and the places an error value reaches another process.

What one package cannot answer alone travels as a `go/analysis` fact on the object it is
about: whether a function blocks and with what bound, whether it starts an unrecovered
goroutine, whether it hands back memory derived from its receiver, whether it replies,
which parameters it forwards into a send, which behavior a factory returns and what its
`Init` waits on, and whether it builds an `fmt.Errorf` value with a `%w` operand. That is
what lets a rule report at the spawn site something that is only decidable inside the
factory's own package.

Every finding goes through one funnel, so severity, suppression, the baseline, the debt
metric and the offered quick fix cannot be forgotten by a new rule. Findings from the
standard library and the module cache are never reported: under `go vet` the tool runs
over the whole build graph to compute facts, and only code the user can edit is worth a
diagnostic.

## Extending

The rule set is composed explicitly in `rules.Suite()`. An external rule set is built the
way golangci-lint composes module plugins: import your own rule packages into your own
main and rebuild.

```go
func main() {
	multichecker.Main(append(rules.Suite(), myrule.Analyzer)...)
}
```

A rule requires `ergomodel.Analyzer`, reads the model through `ergomodel.From(pass)`, and
reports through `Model.Report`. Its analyzer name must be `argus<RuleID>` and the id must
be registered in `ergomodel.KnownRuleIDs`; the suite tests enforce both, plus a fixture
under `testdata/src/<ruleid>` for every rule in the set.

## Development

```bash
go test ./...        # analysistest over testdata, plus the model and config tests
go test -race ./...  # the model is read by parallel rules and the contract is asserted
gofmt -l .
go vet ./...
```

`testdata/src/ergo.services/...` holds a stand-in for the framework surface the rules
resolve against. It mirrors the real declarations on purpose: a stub that drifts makes a
test pass against a surface that does not exist.
