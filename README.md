# mycs-common

Shared Go libraries for Novassist MyCloudSpace–related tooling: **`pkg/goutils`** (logging, crypto, REST, networking helpers) and **`pkg/common`** (events, monitors, node API client types, network helpers, embedded Tailscale wiring).

## Module

```text
module github.com/novassist/mycs-common
```

Consumers (for example **`mycs-client-core`**) should add a `require` and a **local `replace`** while developing from sibling checkouts:

```go
require github.com/novassist/mycs-common v0.0.0

replace github.com/novassist/mycs-common => ../mycs-common
```

## Local replaces (forks / vendored paths)

This module expects the same sibling layout as `mycs-client-core`:

- **`tailscale.com`** → `../tailscale`
- **`github.com/hashicorp/terraform-config-inspect`** → `../terraform-config-inspect`
- **`gvisor.dev/gvisor`** → `./third_party/gvisor` (patched tree in this repo)

## Packages

See [docs/packages.md](docs/packages.md) for import paths and roles.

## Build

From this directory (with the replaces above resolvable):

```bash
go build ./...
```
