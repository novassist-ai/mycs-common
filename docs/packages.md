# Packages (`mycs-common`)

Import prefix: `github.com/novassist/mycs-common/pkg/<area>/<package>`.

## `goutils`

| Path | Role |
|------|------|
| `auth` | OAuth-style authentication helpers and HTTP auth wiring. |
| `crypto` | Keys, VPN-related crypto, RSA/EC helpers for clients. |
| `logger` | Structured logging facade used across the stack. |
| `network` | Routing, DNS, packet filtering (Linux-oriented), interface context. |
| `persistence` | Streaming JSON parsing and durable buffers. |
| `rest` | HTTP client helpers including auth token handling. |
| `run` | CLI execution, interrupts, process lifecycle. |
| `streams` | Writer filters and expectation-style readers for pipes. |
| `term` | Terminal sizing and related helpers. |
| `utils` | General-purpose helpers (files, maps, tasks, timers). |

Example:

```go
import "github.com/novassist/mycs-common/pkg/goutils/logger"

logger.Init(logger.DebugLevel)
logger.Infof("connected")
```

## `common`

| Path | Role |
|------|------|
| `events` | CloudEvents-oriented helpers. |
| `monitors` | Monitoring hooks shared with client features. |
| `mycsnode` | Shared node API client and authentication flow against a space node (`SpaceNode` interface). |
| `network` | Shared networking utilities and bridges to Tailscale. |
| `tailscale` | Embedded-style Tailscale daemon wiring for advanced flows. |

VPN configuration, WireGuard-oriented helpers, and integration test mocks that depend on **`cloudbuilder`** live under **`mycs-client-core/pkg/clientcore`** (`vpn`, `test/mocks`, etc.) so **`mycs-common`** never imports **`mycs-client-core`**.

Prefer importing from the MyCloudSpace client layer when you only need higher-level behavior; import from here when you need these shared types and helpers directly.
