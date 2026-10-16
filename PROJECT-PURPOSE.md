# Project Purpose

This repository is a secure ArgoCD gateway for developer self-service deployment.

The project exists to let developers deploy applications using their own GitHub identity and scoped access, without exposing shared admin credentials or forcing every deployment through a central platform bottleneck.

## What this repo does

`pgate` sits in front of ArgoCD and enforces a simple pattern:

1. Authenticate the user through GitHub
2. Run policy checks such as repo role, CODEOWNERS, security scan status, environment order, and required change records
3. Validate the target ArgoCD app against the repo and environment
4. Open the ArgoCD OIDC flow for the user
5. Proxy the ArgoCD command using a short-lived, scoped token

This means the platform can enforce governance automatically while developers still retain control over their own build, pipeline, and delivery process.

## Why this matters

The problem this repo addresses is not just "how to deploy to ArgoCD." The problem is how to keep deployment fast, auditable, and secure without creating a shared-admin model that blocks teams.

This project supports a self-service model where:

- developers own their repo and pipeline
- platform policy is enforced at runtime
- access is scoped to the right namespace and environment
- ArgoCD audit logs show real user identity
- humans are not the deployment gate

## At a glance

```text
+--------------------------------------------------------------+
|                    Self-service platform structure             |
+--------------------------------------------------------------+

+----------------------+      +-------------------------------+
| Developer layer      |      | Platform control layer        |
|----------------------|      |-------------------------------|
| - Owns repo          |      | - RBAC and namespace         |
| - Owns pipeline      |      |   isolation                   |
| - Owns deploy timing |      | - Runtime policy checks      |
| - Uses GitHub ID     |      | - Security + env validation  |
| - Works in guardrails|      | - Audit logging               |
+----------------------+      +-------------------------------+
            \                           /
             \_________________________/
                         |
                         v
+--------------------------------------------------------------+
|                     Gateway layer (this repo)                 |
|--------------------------------------------------------------|
| - pgate CLI                                                  |
| - GitHub authentication                                      |
| - Policy evaluation                                          |
| - ArgoCD OIDC login                                           |
| - Scoped token issuance                                       |
| - Deployment proxying                                        |
+--------------------------------------------------------------+
                         |
                         v
+--------------------------------------------------------------+
|                     Runtime outcome                             |
|--------------------------------------------------------------|
| - No shared admin credentials                                 |
| - Time-limited access                                         |
| - Policy-gated deploys                                        |
| - ArgoCD actions tied to real user identity                   |
+--------------------------------------------------------------+
```

## Repo focus

This repo is about the technical implementation of the self-service deployment path:

- GitHub-based authentication
- policy enforcement before ArgoCD access
- OIDC identity flow
- CLI-driven command proxying
- environment-aware deployment control

## Claude skill support (request-first experience)

This repo also supports a Claude skill workflow so end users can request deployment actions in plain language, instead of memorizing every CLI command.

```text
+--------------------------------------------------------------+
|                 End-user request layer (Claude)               |
|--------------------------------------------------------------|
| "deploy to dev" | "check status" | "rollback to revision X"  |
+--------------------------------------------------------------+
                         |
                         v
+--------------------------------------------------------------+
|                  Claude skill layer (/pgate)                  |
|--------------------------------------------------------------|
| - Interprets request intent                                   |
| - Maps request to approved pgate operations                   |
| - Reuses repo/env context                                     |
| - Keeps policy-first execution path                           |
+--------------------------------------------------------------+
                         |
                         v
+--------------------------------------------------------------+
|                 pgate gateway layer (this repo)               |
|--------------------------------------------------------------|
| - Auth + policy + OIDC + scoped token + ArgoCD proxy          |
+--------------------------------------------------------------+
```

For end users, the experience is simple: make a request, and the Claude skill drives the approved `pgate` path with governance checks still enforced.

### Example prompts end users can ask

- "Deploy my app to dev using pgate."
- "Check deployment status for payment-service in staging."
- "Run a dry-run deploy to prod and show policy check results."
- "Show rollback history for payment-service-prod."
- "Rollback payment-service-prod to revision 3."
- "Sync payment-service-staging and wait until healthy."
- "Show logs for payment-service-dev."

It is not the full design document for platform governance. That conceptual discussion lives in [PAAS Self-Service.md](./PAAS%20Self-Service.md).

## Related docs

- [PAAS Self-Service.md](./PAAS%20Self-Service.md) — platform design and governance direction
- [CASE-STUDY-ARGOCD-DEPLOYMENT-WITH-PGATE.md](./CASE-STUDY-ARGOCD-DEPLOYMENT-WITH-PGATE.md) — example of the deployment flow using `pgate`
- [README.md](./README.md) — command-level usage and setup

## In one sentence

This repo is the implementation of a safer, more scalable deployment model: developers self-serve within guardrails, and the platform enforces those guardrails automatically.
