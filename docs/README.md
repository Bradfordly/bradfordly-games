# Documentation

Design lives in two trees. Specs explain the product. ADRs record the decisions those specs implement.

- Specs: [specs/](specs/)
- Decisions: [ADRs/](ADRs/)
- New file outlines: [templates/adr.md](templates/adr.md), [templates/spec.md](templates/spec.md)

Agent operating rules are in [AGENTS.md](../AGENTS.md). The operator agent keeps this tree organized ([ADR-0007](ADRs/ADR-0007-operator-agent.md)).

## Specs

| Spec | Role |
| --- | --- |
| [system-overview.md](specs/system-overview.md) | Map of the product, components, and defaults |
| [control-plane.md](specs/control-plane.md) | Authenticated panel and API |
| [edge-gateway.md](specs/edge-gateway.md) | Wake, proxy, idle stop |
| [game-adapters.md](specs/game-adapters.md) | Per-title behavior |
| [operations.md](specs/operations.md) | Hosting, cost model, runbooks |

## ADRs

| ADR | Status |
| --- | --- |
| [ADR-0001](ADRs/ADR-0001-prior-art-and-viability.md) | Accepted |
| [ADR-0002](ADRs/ADR-0002-reject-wings-use-eks-fargate.md) | Superseded by ADR-0006 (no Wings still stands) |
| [ADR-0003](ADRs/ADR-0003-edge-gateway-first.md) | Accepted (runtime amended by ADR-0006) |
| [ADR-0004](ADRs/ADR-0004-persistence-and-networking.md) | Superseded by ADR-0006 |
| [ADR-0005](ADRs/ADR-0005-identity-for-games-bradfordly.md) | Accepted |
| [ADR-0006](ADRs/ADR-0006-pack-on-public-ec2.md) | Proposed |
| [ADR-0007](ADRs/ADR-0007-operator-agent.md) | Accepted |
