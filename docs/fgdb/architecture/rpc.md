> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# pkg/rpc, pkg/security, and authentication

Nodes talk to each other over gRPC. Clients talk SQL over pgwire, on the same default port, and the console over HTTP. This page is how those connections are authenticated and how the process decides the clocks are close enough.

## Purpose

- `pkg/rpc`: dial peers, heartbeat them, measure clock offset, and expose the gRPC server the KV layer uses.
- `pkg/security`: load and rotate the certificate files (`ca.crt`, `node.crt`, `client.<user>.crt`).
- `pkg/sql/pgwire`: authenticate the SQL session (password, SCRAM, client cert, trust, reject).
- `pkg/server/authserver`: the HTTP login used by DB Console, including the OIDC hook.

## Key types and entry points

| Piece | Where | Role |
| --- | --- | --- |
| `rpc.Context` | `pkg/rpc/context.go` | Clock, settings, security context, heartbeats. Built in `server.NewServer` via `rpc.NewContext`. |
| `RemoteClockMonitor.VerifyClockOffset` | `pkg/rpc/clock_offset.go` | Fails when this node is outside the tolerated offset of at least half the nodes it has measured. |
| Heartbeat apply | `pkg/rpc/peer.go` | `updateClockOffsetTracking`. On error, `log.Ops.Fatalf` if `FatalOnOffsetViolation` is set. |
| TLS | `pkg/rpc/tls.go` | `GetServerTLSConfig`. Insecure mode returns nil TLS. |
| `CertificateManager` | `pkg/security/certificate_manager.go` | Loads CA, node, and client certs. Reloads on SIGHUP (`RegisterSignalHandler` from `PreStart`). |
| `loadDefaultMethods` | `pkg/sql/pgwire/auth_methods.go` | Built-in HBA methods. |
| `RegisterAuthMethod` | `pkg/sql/pgwire/hba_conf.go` | Extra methods can be added at init. The comment says CCL does this. |
| `ConfigureJWTAuth` | `pkg/sql/pgwire/auth_methods.go` | Defaults to `noJWTConfigured`. |
| `ConfigureOIDC` | `pkg/server/authserver/authentication.go` | Defaults to `noOIDCConfigured` (`Enabled: false`). |

`cli cert` (`pkg/cli`) creates the files the `CertificateManager` expects. The operations page has a copy-paste lab for that. This page does not repeat it.

## How data and control enter and leave

**Node to node.** `DistSender` and Raft dial through `rpc.Context`. Each connection heartbeats. The heartbeat carries the server's clock. `updateClockOffsetTracking` compares it with the local HLC and calls `VerifyClockOffset`.

The tolerated offset is `0.8 * MaxOffset` (`ToleratedOffset` in `pkg/server/config.go`), unless `--disable-max-offset-check` is set, in which case the check returns 0 and `VerifyClockOffset` does nothing. The default max offset is 500 ms (`base.DefaultMaxClockOffset`). The flag parser rejects a max offset above 5 s.

The error string, when the check fails, is:

```text
clock synchronization error: this node is more than %s away from at least half of the known nodes (%d of %d are within the offset)
```

`peer.go` turns that into `log.Ops.Fatalf` when fatal-on-violation is on. The comment on `VerifyClockOffset` says a non-nil error means the clock is unreliable and the node should terminate. An uncertain measurement (the tolerated offset sits inside the ping's uncertainty window) is treated as healthy, with a health-log warning, so a slow RPC does not kill the node by itself (`RemoteOffset.isHealthy`).

**SQL client.** pgwire runs `handleAuthentication` before `sql.Server.ServeConn`. Methods registered in `loadDefaultMethods`:

| Method | Notes |
| --- | --- |
| `password` | Clear-text password, compared with the stored hash. |
| `scram-sha-256` | SCRAM. |
| `cert` | TLS client cert, `ConnHostSSL` only. |
| `cert-password`, `cert-scram-sha-256` | Cert plus a password check. |
| `trust` | No check. This is what `--insecure` labs use. |
| `reject` | Always fails. |

HBA rules (`pg_hba.conf`-style, cluster setting `server.host_based_authentication.configuration`) pick the method. `--insecure` is `base.Config.Insecure`: no TLS on the RPC listener, and the HTTP server is marked insecure.

**HTTP.** DB Console login is `pkg/server/authserver`. OIDC is a hook. The OSS default returns a config with `Enabled: false` and does not error at startup.

## What it depends on

- `pkg/util/hlc` for the local clock the offset is compared against.
- `pkg/security` for the certificate manager the RPC context holds.
- `pkg/settings` for HBA configuration and web session timeout (`server.web_session_timeout` in `authentication.go`).
- gRPC and cmux (the listener split is in [server.md](server.md)).

JWT, OIDC, and GSSAPI packages exist under `pkg/ccl` (`jwtauthccl`, `oidcccl`, `gssapiccl`). Their `init` functions are what would replace these hooks. This page does not describe those packages. The OSS defaults are above.

## Invariants

- A node certificate is the identity other nodes expect. Changing the cert without the CA that signed it drops RPC, which drops Raft, which looks like a down replica.
- Clock checks use heartbeats between nodes. A SQL client with a wrong clock does not trip `VerifyClockOffset`. The client's transaction still gets uncertainty restarts if its observed timestamps disagree with the server. That is the HLC, not this fatal check.
- `--insecure` and `trust` are the same idea: no authentication. Do not combine them with a publicly reachable port.
- Cert reload on signal does not restart the process. Existing connections keep the old TLS state until they reconnect.

## Gotchas

- **JWT looks configured and then fails.** `authJwtToken` always builds a verifier. The OSS `ConfigureJWTAuth` returns `noJWTConfigured`, whose `ValidateJWTLogin` returns `JWT token authentication requires CCL features`.
- **OIDC does not fail startup.** `ConfigureOIDC` returns `noOIDCConfigured` with `Enabled: false`. The console has no SSO button. That is not an error in the log.
- **GSSAPI is not in `loadDefaultMethods`.** A search of `pkg/sql/pgwire` found no `RegisterAuthMethod("gss")`. The directory `pkg/ccl/gssapiccl` is the other binary's hook. Whether a build tag in this tree registers it from somewhere else is unchecked; see [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md).
- **`cert` without TLS is rejected by the method's connection type** (`hba.ConnHostSSL`), not by a later SQL error.
- **Fatal clock skew takes the process down.** It does not drain. A lab with suspended VMs (laptops sleeping) hits this. The fix is to keep clocks stepped, not to raise the offset past what your writes can tolerate. `MaxOffset` is also the uncertainty window for transactions (`Config.MaxOffset` comment in `pkg/server/config.go`).
- **Cluster name mismatch** fails the ping (`checkClusterName` in `peer.go`) before the offset check. A node started with the wrong `--cluster-name` never joins, which is what you want.
