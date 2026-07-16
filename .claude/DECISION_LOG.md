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
