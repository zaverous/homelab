# DECISION_LOG.md — Architecture Decision Records

ADRs for the homelab K3s project. Newest at the bottom. Keep each entry short —
its value is that it exists and shows the reasoning, not its length.

## Template

```
## ADR-NNN: <short decision title>
- Date: YYYY-MM-DD
- Status: Proposed | Accepted | Superseded by ADR-XXX

### Context
What forced a decision? What constraints applied (hardware, budget, time, network)?

### Decision
What we chose.

### Alternatives considered
Options rejected, and why.

### Consequences
What we gained, what we gave up, what we would revisit at scale.
```

---

## ADR-001: Treating Mixed-Architecture (arm64/amd64) as a Feature, Requiring Multi-Arch Builds or Explicit Node Scheduling
- Date: 2026-07-16
- Status: Accepted

### Context
The cluster spans two CPU architectures: 2× arm64 (`control-plane-01` Pi 5,
`worker-node-01` Pi 3B+) and 1× amd64 (`debian-server-hp` HP x2 210 G2). A
container image built for a single architecture fails to run (`exec format error`)
on nodes of the other architecture. Rather than homogenize the hardware — which
would waste the amd64 node and the learning opportunity — we treat the
heterogeneity as a deliberate, showcaseable portfolio feature.

### Decision
Mixed-arch is a first-class constraint. Every custom image MUST either:
1. Be a multi-arch build (`docker buildx build --platform linux/arm64,linux/amd64`,
   pushed as a manifest list to GHCR), or
2. Carry explicit scheduling pinning it to architecture-compatible nodes
   (`nodeSelector: kubernetes.io/arch: <arch>`, node affinity, or a matching
   label/taint).

No workload may rely on an implicit or default architecture.

### Alternatives considered
- **Homogenize to a single arch (drop the amd64 node or the Pis):** rejected —
  wastes hardware, removes the amd64 workhorse, and discards a strong "I run
  heterogeneous infra deliberately" interview story.
- **Pin everything to one arch, leave other nodes idle:** rejected — defeats the
  purpose of a 3-node cluster and starves an already RAM-scarce cluster of capacity.
- **Assume upstream images are multi-arch:** rejected as policy — most public
  images are, but "hope" is not a control; we verify the manifest list or pin.

### Consequences
- Gained: workloads are portable across all nodes; the Phase 2 CI/CD pipeline must
  produce multi-arch images — a valuable, demonstrable skill.
- Gave up: build complexity — `buildx` / QEMU emulation is slower, and CI must set
  up multi-platform builders.
- Revisit at scale: with many nodes of one arch, per-arch node pools + affinity may
  beat universal multi-arch builds on build-time cost.

---

## ADR-002: FluxCD over Argo CD for GitOps
- Date: 2026-07-16
- Status: Accepted

### Context
Phase 2 needed a GitOps controller to take manual `kubectl`/SSH out of day-to-day
changes — push to git, the cluster reconciles itself. The binding constraint is
memory: the reconciler's controllers have to run on this cluster alongside
everything else, and `worker-node-01` (Pi 3B+, 905MB) has essentially no headroom.

### Decision
Use **FluxCD**, bootstrapped against `github.com/zaverous/homelab`
(`flux bootstrap github`, path `clusters/homelab`). Its controllers
(`source`/`kustomize`/`helm`/`notification`) run as a set of lightweight
pods and landed on `debian-server-hp` on their own — the `PreferNoSchedule` taint
kept them off the Pi 3B+ without extra config.

### Alternatives considered
- **Argo CD:** rejected for this cluster — heavier footprint (application-controller,
  repo-server, Redis, plus the web UI/API server) than Flux's controllers, and the
  UI's value doesn't justify the RAM here. Argo's UI is stronger for teams; this is
  a solo homelab where the git repo is already the source of truth.
- **No GitOps (stay on `kubectl`/`helm` by hand):** rejected — reproducibility and
  drift were exactly the problems Phase 2 existed to solve.

### Consequences
- Gained: small resource footprint, git as the single source of truth, automatic
  reconciliation and pruning.
- Gave up: Argo's out-of-the-box visualization/UI; Flux is more CLI/manifest-driven,
  a slightly steeper first-run learning curve.
- Revisit at scale: Argo CD if a team or a shared UI/RBAC surface is ever needed.

---

## ADR-003: SOPS + age over sealed-secrets for git-committable secrets
- Date: 2026-07-17
- Status: Accepted

### Context
The repo is **public**, so any secret committed to git must be encrypted at rest.
Phase 3 (cert-manager, Cloudflare Tunnel) needs credentials delivered
GitOps-natively. The cluster is also RAM-scarce, so an always-on decryption
controller is a real cost.

### Decision
Use **SOPS + age**. Flux's `kustomize-controller` decrypts SOPS natively — **no
extra pod**. The age private key lives as the `sops-age` Secret in `flux-system`
(and a workstation copy at `%AppData%\sops\age\keys.txt`), never in git; the public
key sits in `.sops.yaml` at repo root, whose rule encrypts `*.enc.yaml` files over
the `^(data|stringData)$` regex. Kustomizations that carry secrets set
`decryption.provider: sops`.

### Alternatives considered
- **sealed-secrets (Bitnami):** rejected — requires an always-on controller pod
  (RAM we can't spare) that also holds the sealing key; SOPS needs neither.
- **Plaintext / private repo:** rejected — the repo is deliberately public for
  portfolio value; secrets must be encrypted regardless.
- **External secret manager (Vault, cloud KMS):** rejected as over-engineering for
  a homelab — more infrastructure to run than the problem warrants at this stage.

### Consequences
- Gained: zero extra runtime components, native Flux integration, encrypted files
  committed openly and safely.
- Gave up: manual age-key management (documented as living in exactly two places),
  plus a Windows key-path gotcha (`%AppData%\sops\age`, not `~/.config/sops/age`).
- Revisit at scale: an external secret store (Vault / cloud KMS) if the team or the
  secret surface grows.

---

## ADR-004: Full (strict) end-to-end TLS termination
- Date: 2026-07-17
- Status: Accepted (supersedes the edge-only setup used during 3c validation)

### Context
Public exposure runs through Cloudflare Tunnel (chosen because egress is
IPv6/CGNAT, so no inbound ports). Cloudflare can terminate TLS at its edge, which
overlaps with the cert-manager + Let's Encrypt capability built in 3b — so the
termination posture is a real choice, not a given.

### Decision
Adopt **Full (strict)**: Cloudflare terminates TLS at its edge with its own cert,
AND cert-manager issues a Let's Encrypt origin cert (`letsencrypt-prod`) that
Traefik serves on `websecure`; `cloudflared`→Traefik runs over HTTPS with an
`originServerName` SNI override (the connection URL is the internal cluster
address, which doesn't match the cert name, so the override is required and TLS
verification stays **on**). Every hop — browser↔edge, edge↔tunnel, tunnel↔Traefik
— is encrypted with a verified cert.

### Alternatives considered
- **Edge-only (Cloudflare terminates, in-cluster hop plaintext):** simplest, and
  is what 3c used to first validate the tunnel. Rejected as the end state — the
  internal hop being cleartext undercuts the whole point of the exercise.
- **Full (non-strict) (encrypted but unverified origin):** rejected — encrypts the
  hop but doesn't verify the cert, so it adds little real security over edge-only.
- **Origin-only (no edge termination):** not applicable with Cloudflare Tunnel.

### Consequences
- Gained: genuine end-to-end encryption with verified certs, not just TLS
  terminated at Cloudflare's edge — the security story an interviewer can probe.
- Gave up / accepted: more moving parts (a prod issuer, a `Certificate` per host)
  and one **manual, non-git-managed step** — setting Service `HTTPS` + Origin
  Server Name in the Cloudflare Tunnel Public Hostname config — plus care against
  Let's Encrypt production rate limits.
- Revisit: fine as-is; the pattern (cert + `websecure` IngressRoute + tunnel
  Origin Server Name) is now the reusable template for every new public hostname.

---

## ADR-005: Self-hosted Postgres + Redis on local-path, pinned to the Pi 5
- Date: 2026-07-17
- Status: Accepted

### Context
KubePets (Phase 4) needs persistent pet state (Postgres) and a job queue / message
broker (Redis). The cluster's only StorageClass is `local-path` — node-local, no
replication — and a `local-path` PV physically binds to whichever node its consuming
pod first schedules onto. The three nodes differ sharply in storage: Pi 3B+
(`worker-node-01`, 905MB, SD card — slow, wear-prone), HP (`debian-server-hp`,
3.7GB, eMMC), Pi 5 (`control-plane-01`, 4GB, NVMe SSD). We need durable,
reasonably fast storage for a write-touching database.

### Decision
Run Postgres and Redis in-cluster as single-replica StatefulSets, **both pinned to
`control-plane-01` (Pi 5)** via required hostname `nodeAffinity`, on `local-path`
PVCs — which therefore land on the Pi 5's NVMe. Redis is capped
(`--maxmemory 192mb --maxmemory-policy noeviction`) so a runaway queue can't OOM
the node; a full queue applies backpressure to producers instead. No managed
database at this stage — that contrast is deliberately saved for Phase 5 (GKE
Cloud SQL).

### Alternatives considered
- **DB on `debian-server-hp` (most free RAM):** rejected — eMMC is much slower than
  NVMe for a write-heavy DB, and we specifically want the fast disk.
- **DB on `worker-node-01` (Pi 3B+):** rejected outright — 905MB and an SD card;
  it would OOM and wear the card.
- **Longhorn / replicated storage so the DB survives node loss:** rejected —
  RAM-heavy and needs healthy multi-node replicas; too costly on this hardware
  (consistent with the earlier local-path-over-Longhorn reasoning).
- **Managed cloud DB now:** rejected — defeats the bare-metal "operate stateful
  workloads on metal, own the failure modes" story; kept as the Phase 5 contrast.

### Consequences
- Gained: fast NVMe-backed storage, minimal moving parts, zero extra components,
  and full ownership of the failure modes — the reliability narrative.
- Accepted risk: node-local, non-replicated durability. If `control-plane-01`'s
  disk fails or the node is lost, the DB is unavailable until it returns or is
  restored from backup. This also **co-locates the app DB on the same node as the
  k3s datastore, which is already the cluster SPOF** — concentrating blast radius
  on one node. Mitigation: `pg_dump` backups (to add) plus honest documentation of
  the failure mode (which doubles as the chaos / post-mortem material).
- Revisit at scale: replicated storage (Longhorn) or a managed DB once RAM/nodes
  allow or durability requirements harden.

---

## ADR-006: Queue-driven autoscaling — KEDA (Redis listLength) over plain CPU HPA
- Date: 2026-07-17
- Status: Proposed (decision lands at Phase 4b/4c)

### Context
KubePets' worker tier must scale in response to Redis job-queue depth: a CronJob
enqueues ~10k "calculate hunger" events/min, workers `BRPOP` and process them, and
the endgame drives the Pi 3B (1GB) into memory pressure. Kubernetes' built-in
HorizontalPodAutoscaler can only scale on resource metrics (CPU/memory via
metrics-server) or on custom/external metrics via an adapter — it **cannot** natively
read "length of a Redis list."

### Decision
Use **KEDA** with its Redis `listLength` scaler to drive worker replica count from
actual queue depth. KEDA provisions and manages a standard HPA under the hood — so
the "I configured an HPA" story holds — but feeds it the semantically correct
signal (queue length), matching the intent "spike the queue → workers scale." Run
the KEDA operator/metrics-adapter on `debian-server-hp` (it must not compete for
RAM on the Pi 3B).

### Alternatives considered
- **Plain HPA on CPU utilization:** simplest, no new component; busy workers spike
  CPU, so CPU is a usable proxy. Kept as the zero-dependency fallback if KEDA proves
  too heavy, but it scales on a proxy, not the queue, so it tells the story less
  cleanly.
- **Plain HPA on memory:** rejected — memory-based scaling conflicts with the OOM
  endgame (scaling up as memory climbs muddies the very signal we want to blow past)
  and memory is a poor proxy for backlog.
- **Prometheus-adapter custom metric (export queue length → HPA):** viable and
  reuses the Phase 4d observability stack, but more wiring than KEDA for the same
  result; becomes more attractive once `kube-prometheus-stack` is in anyway.

### Consequences
- Gained (KEDA): correct queue-driven autoscaling, optional scale-to-zero, an
  idiomatic and interview-strong choice — and it still yields a real HPA object.
- Gave up: one more operator to run and reason about (RAM on `debian-server-hp`, a
  second autoscaling abstraction to debug).
- Revisit: fall back to CPU HPA or a prometheus-adapter metric if KEDA is too heavy
  once 4d lands.

---

## ADR-007: Stateless JWT cookie sessions over server-side session storage
- Date: 2026-07-18
- Status: Accepted

### Context
KubePets gained Google OAuth user accounts, and the platform plan explicitly
subjects API pods to random OOM kills (chaos demos) with the requirement that
users must never be logged out by a pod death. Sessions therefore cannot live
in pod memory. The realistic options: server-side sessions in Postgres, or
self-contained signed tokens.

### Decision
Stateless **HS256 JWT session cookies** (`kp_session`, HttpOnly, SameSite=Lax,
7-day expiry). The token carries the user's id/email/name/picture; every API
replica shares `JWT_SECRET` (SOPS-encrypted Secret `kubepets-oauth`), so any
pod validates any request with zero session state and zero DB reads. The OIDC
issuer is configurable (`OIDC_ISSUER`, default Google) so the full login flow
is testable against a local fake provider without real Google credentials.
Auth degrades gracefully: if the secret still holds `REPLACE_ME` placeholders,
the API boots with auth disabled rather than crash-looping — the deployment
never depends on the Google OAuth client existing.

### Alternatives considered
- **Sessions table in Postgres:** also survives pod death and is revocable,
  but adds a DB round-trip to every request and gives the already-SPOF'd
  Postgres (ADR-005) a hot path for every page load. Rejected here; the right
  upgrade if revocation ever matters.
- **In-memory sessions + sticky routing:** violates the stated constraint
  outright (OOM kill = mass logout) and fights the HPA. Rejected.
- **Redis-backed sessions:** Redis is deliberately the chaos target
  (noeviction, queue floods) — storing auth state in the component we
  intentionally saturate would couple login to the failure demo. Rejected.

### Consequences
- Gained: pods are fully stateless (the OOM-kill demo can't log anyone out),
  no per-request session I/O, trivially horizontal.
- Gave up: server-side revocation — a stolen/stale session stays valid until
  expiry (7d). Acceptable for a tamagotchi; mitigations if ever needed: short
  expiry + refresh, or a small denylist.
- Note: `/chaos/batch-feed` requires a session when auth is configured — once
  the frontend goes public (4f), the flood button must not be anonymous.
