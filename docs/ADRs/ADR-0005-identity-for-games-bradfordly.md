# ADR-0005: Identity for games.bradfordly.com

## Status

Accepted

## Date

2026-10-03

## Context

The control plane is public at `games.bradfordly.com` and must require a login to view. The panel is not a public multi-tenant host. It is a personal or small-community console for Bradfordly and invited friends.

Game join is a separate problem. Minecraft whitelist, Valheim password, and Palworld password already exist. Putting the panel's identity provider in front of UDP game traffic is not practical.

Options considered:

- **Open registration:** rejected. The panel would become a public hosting product.
- **Shared password / HTTP basic:** simplest, but no per-user audit and a poor fit for "require a login."
- **OIDC with an allowlist:** GitHub or Google sign-in, then a server-side list of permitted subjects. Matches invite-only. No user database to start.
- **Amazon Cognito user pool:** works on AWS, more moving parts than an allowlist in front of a generic OIDC provider.
- **API keys only:** fine for the gateway-to-control-plane path, not for humans in a browser.

## Decision

1. **The panel requires authentication on every page and API.** Unauthenticated requests to `games.bradfordly.com` redirect to the identity provider (browser) or receive 401 (API).
2. **v1 identity is OIDC** (GitHub or Google) plus a **server-side allowlist** of email addresses or issuer subject IDs. People not on the list authenticate at the provider and are still denied.
3. **v1 is invite-only, not multi-tenant.** There are no orgs, teams, or per-world ACLs beyond "allowlisted user can see and operate every world." If a later revision needs friend-only worlds, add RBAC then.
4. **Game join does not use panel login.** Each world's own whitelist or password remains the join gate. The gateway may optionally refuse to wake a Minecraft world unless the login name is on that world's whitelist (lazymc's `wake_whitelist` idea). That is an adapter setting, not panel SSO.
5. **Service-to-service calls** (gateway to control plane, control plane to Kubernetes) use Kubernetes service accounts and in-cluster configuration, not the human OIDC flow.
6. **Session cookies are HTTPS-only** on `games.bradfordly.com`. Do not put panel credentials in game-server environment variables beyond what the dedicated-server image already needs.

## Consequences

- Someone must edit the allowlist (config map, environment, or a tiny table) to invite a friend. There is no self-serve "request access" in v1.
- Choosing GitHub versus Google is an implementation detail in [docs/specs/control-plane.md](../specs/control-plane.md). Either is acceptable if the allowlist key is stable.
- Lost-device / account-recovery is the identity provider's problem.
- Public MOTD and "server is asleep" messages on the game ports are not authenticated. That is intentional: players need to see them without opening the panel.
- Audit of who woke a world can use the game login name (Minecraft) or "unknown UDP join" (Valheim/Palworld) plus the panel user for UI-triggered starts.
