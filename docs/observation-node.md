# Dedicated destination observation (opt-in)

The panel may enable `observation_audit` only for a dedicated, Xray-backed
`v2node` node. Do not enable it on a shared production node. The config must
contain an explicit `allowed_user_ids` list; an empty or invalid list makes
the node fail closed. The controller filters the node's authentication user
list to those IDs, rather than relying on subscription delivery rules.

The panel issues a distinct 64-character observation token. Install it in the
node's local config as `ObservationToken`; it is **not** the ordinary `ApiKey`
and must not appear in the panel's node config response. The node sends it in
`X-Observation-Token` to `/api/v1/server/UniProxy/observation`.

When enabled, the Xray dispatcher reports one metadata event per authenticated
routing attempt: user ID, time, source IP, visible target hostname or target
IP, target port and TCP/UDP. `result=route_attempt` deliberately **does not**
claim that the website was reached. Full URLs, page contents and TLS payloads
are never recorded. A hostname may be unknown when a client sends an IP and
sniffing cannot identify the domain.

Events are batched in sets of at most 100 with stable IDs for server-side
deduplication. Failed reports remain in an in-memory bounded retry queue;
the oldest events are discarded on overflow and the loss is logged. The queue
does not survive a process restart. Deploy and smoke-test with one test user
first; do not interpret an empty panel as proof of no browsing.

The feature is not implemented for embedded/external protocols (Juicity,
Mieru, Naive). Those protocols fail closed if observation is enabled.
