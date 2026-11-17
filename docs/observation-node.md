# Dedicated destination observation (opt-in)

The panel can opt in individual Xray-backed `v2node` nodes to
`observation_audit`. `mode: "all"` observes every authenticated user of that
node and **does not change its user list**; it is not a global switch, so
full-network coverage requires all serving nodes to support and enable it.
Absent `mode` or `mode: "allowlist"` preserves the older dedicated-node
behavior: the config must contain an explicit `allowed_user_ids` list, and
the controller filters node authentication to those IDs. An empty list while
enabled fails closed. Invalid modes fail closed. External/embedded protocols
such as Juicity, Mieru and Naive cannot provide this Xray dispatcher event
and are rejected rather than being counted as covered.

The panel issues a distinct 64-character observation token. Install it in the
node's local config as `ObservationToken`; it is **not** the ordinary `ApiKey`
and must not appear in the panel's node config response. The node sends it in
`X-Observation-Token` to `/api/v1/server/UniProxy/observation`.
This node advertises `X-Observation-Capability: all-v1` when fetching its
configuration. The panel rejects all-user config requests from older nodes
without that header so an empty list cannot be mistaken for revoking users.

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
Mieru, Naive). Those protocols fail closed if observation is enabled. Even on
supported nodes, events can be lost on queue overflow or process restart;
the records are route attempts, not confirmed website visits.
