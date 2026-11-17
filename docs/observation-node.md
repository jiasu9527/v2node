# Destination access statistics (automatic on supported nodes)

On a current `v2node`, every Xray-backed node collects destination metadata
for **all authenticated users by default**. Updating the node is enough; no
per-node user allowlist or separate observation token is required. The node
advertises `X-Observation-Capability: auto-v2` when fetching its config. The
panel can explicitly turn a node off with `observation_audit.enable=false`;
otherwise an omitted `observation_audit` means enabled all-user mode. An
explicit legacy `observation_audit` without `mode: "all"` continues to use its
dedicated-node allowlist for compatibility.

Reports go to `/api/v1/server/UniProxy/observation`, authenticated with the
node's existing `ApiKey` in the `X-Server-Token` header. The key is never added
to this report URL. An old `ObservationToken` config field is tolerated but
not used by this report path. The panel must accept `auto-v2` and this header
before deploying automatic collection; otherwise reports will remain queued
and eventually overflow.

The Xray dispatcher reports one metadata event per authenticated
routing attempt: user ID, time, source IP, visible target hostname or target
IP, target port and TCP/UDP. `result=route_attempt` deliberately **does not**
claim that the website was reached. Full URLs, page contents and TLS payloads
are never recorded. A hostname may be unknown when a client sends an IP and
sniffing cannot identify the domain.

Events are batched in sets of at most 100 with stable IDs for server-side
deduplication. Failed reports remain in an in-memory bounded retry queue;
the oldest events are discarded on overflow and the loss is logged. The queue
does not survive a process restart. The panel should expose last-report time,
event counts and gaps so an empty statistics page is not mistaken for no
browsing.

The feature is not implemented for embedded/external protocols (Juicity,
Mieru, Naive). These nodes are **not** automatically enabled and cannot be
counted as covered. Even on supported nodes, events can be lost on queue
overflow or process restart; records are route attempts, not confirmed website
visits.
