# PaaS Self-Service: Platform Direction Change

> This document explains the platform design concept and governance direction behind the project. For the repo-specific implementation and the practical ArgoCD deployment example, see [PROJECT-PURPOSE.md](./PROJECT-PURPOSE.md) and [CASE-STUDY-ARGOCD-DEPLOYMENT-WITH-PGATE.md](./CASE-STUDY-ARGOCD-DEPLOYMENT-WITH-PGATE.md).

## Objective

> Developers should be able to go from code to production using their own code, their own tools, their own pipeline, their own schedule, and their own permissions — operating within **governance guardrails** set automatically, with no human gates and no extra restrictions beyond policy.

This is not a tool change. It is a **transfer of ownership and control** back to developers. The platform team's role shifts from *operating tools and approving requests* to **building the policies and catalog that make self-service safe**.

The project design centers on a self-service platform model where the developer owns their repo, pipeline, and deployment rhythm, while the platform enforces identity, RBAC, namespace isolation, and deployment policy at runtime. In this model, governance is codified in automation rather than embedded in manual approvals, so teams can move quickly without giving up security or compliance boundaries.

To support that model, the Claude skill package provides a guided operational layer for the platform workflow. Through the `pgate` skill, Claude Code can initialize ArgoCD applications, validate deployment policy, enforce GitHub identity and OIDC requirements, and help developers run deployment and rollback commands in the approved self-service path without bypassing the governance controls built into the platform.

---

## The Problem — Current State

Our platform team is operating under a **central control model**. This was a reasonable starting point, but it has become a bottleneck as the organization scales.

Symptoms we are seeing today:

- Developers raise tickets to change a shared library, a pipeline step, or a tool config — and wait
- The platform team owns Jenkins, GitHub Actions runners, and other CI tools, creating tool-admin burden
- **No real RBAC exists** — access is split between a shared admin account and restrictions embedded in pipeline logic
- A single shared pipeline is used across many teams, making change slow and risky for everyone
- Governance is enforced by **human approval** and **pipeline gate steps**, not by automated policy
- Because there is no namespace or role isolation, the only way to prevent someone from doing something wrong is to control what the pipeline allows — which is fragile and easily bypassed

**The result:** The platform team is in the critical path of every team's daily work. Access control is a fiction built on pipeline steps, not enforced by the platform runtime. We are not a platform — we are a bottleneck with a false sense of security.

---

## New Direction — Developer Ownership Within Governance Guardrails

### Core Principle

> Give developers: their code, their time, their RBAC, their tools, their pipeline.
> Give governance: automated policy enforcement at the platform runtime layer.

**Governance defines the rules. The platform team implements them technically.** The platform team does not invent restrictions — it translates governance requirements into runtime policy that is enforced automatically. Developers operate freely within that boundary.

If governance has not requested it, the platform team should not restrict it. The only thing the platform team owns by default — without a governance request — is **RBAC control**: ensuring every identity is scoped, every service account is least-privilege, and every team is isolated within their namespace.

The platform team's job is to make the **right thing easy** and the **wrong thing impossible** — not to hold the pen for every team.

---

### What the Platform Team Owns (and only this)

The items below are **defined by governance** and **implemented by the platform team** as automated, runtime-enforced controls. The platform team does not own the policy decision — only the technical implementation.

```
Governance defines → Platform implements:
  - Network policy baselines
  - OPA / Kyverno admission policies
  - Approved base image registry
  - Resource quota boundaries per team/namespace
  - Audit logging (non-negotiable, always on)
  - Secret backend access pattern (Vault API, not secret content)
  - Deployment gate: image signed? SBOM present? scan passed?
```

### What the Developer Owns

```
Developer Autonomy Layer:
  - Pipeline definition (YAML in their own repo)
  - RBAC within their namespace
  - Tool choice (within the approved catalog)
  - Library versions and upgrade schedule
  - Deployment timing
  - Observability dashboards
```

---

## Pipeline Tool Problem

### Tool Migration Is Not Platform Modernization

The team moved from **Jenkins to GitHub Actions**. This feels like progress, but the underlying model did not change:

```
Jenkins (was):
  - Platform team owns the Jenkins instance
  - Shared pipeline library controlled by platform team
  - Developer requests pipeline changes via ticket
  - Central admin controls what runs and when

GitHub Actions (is now):
  - Platform team owns the self-hosted runners
  - Shared reusable workflows controlled by platform team
  - Developer requests workflow changes via ticket
  - Central admin controls which workflows are allowed
```

**The tool changed. The power structure did not.**

This is the **tool-admin trap**: the platform team absorbs the operational cost of each new tool while developers remain blocked on the same team for changes. Migrating to the next tool (Tekton, Dagger, etc.) will produce the same result unless the model changes.

The question is not *"are we on a modern tool?"* — it is *"do developers own their pipeline, or do they borrow it from us?"*

### The Fix: The Pipeline Belongs to the Developer

The platform team should have **no opinion on the pipeline tool**.

> A pipeline is how a developer thinks, works, and ships. It is personal to their team and their codebase. The platform has no business owning it.

```
Platform team's pipeline responsibility:
  → Zero.

Developer's pipeline responsibility:
  → Everything. Their repo, their YAML, their tool, their schedule.
```

The only thing the platform provides is the **deployment gate** — a check that runs when the developer's artifact arrives at the registry or cluster. The gate does not care how the artifact was built.

**The Developer's Deployment Contract** (what the platform gate checks — automatically, not manually):

| What the gate checks | How it is enforced |
|---|---|
| Code was scanned (SAST/SCA) | Admission webhook — no scan result = rejected |
| Container image is signed | Policy engine rejects unsigned images at deploy time |
| Tests passed | Image not promoted without test result attestation |
| Deployed to correct environment in correct order | ArgoCD promotion policy / environment gate |

The developer uses whatever pipeline tool they prefer — or no dedicated tool at all.
**The gate does not care. It checks the artifact, not the process.**

If the artifact passes → deploy.
If it fails → rejected at the platform boundary, not by a human reviewer.

---

## Comparison: Old Model vs New Model

| Area | Old (Central Control) | New (Developer Ownership) |
|---|---|---|
| Pipeline | Platform team owns and manages | Developer defines in their own repo |
| Shared library | Request ticket, wait for platform team | Developer owns version, platform publishes policy SDK |
| Access control | **No real RBAC** — pipeline logic gates access | Kubernetes RBAC per namespace, per service account |
| Identity | Shared admin account or shared pipeline credential | Each team has a scoped service account with least privilege |
| Isolation | None — all teams share the same trust boundary | Namespace-per-team with network policy and quota |
| Tool selection | Platform team decides and operates | Developer picks from approved catalog |
| Governance enforcement | Pipeline step + human approval (bypassable) | Admission policy enforced at runtime (not bypassable) |
| Pipeline tool | Platform supports specific tools | Platform enforces outcome, tool is developer's choice |
| Audit trail | Pipeline log (tied to job, not identity) | Every action tied to a real service account identity |

---

## What the Platform Team Builds Instead

Rather than operating tools, the platform team builds:

1. **Service Catalog** — discoverable, self-service provisioning for DBs, queues, caches, certs
2. **Policy SDK** — a library developers call from any pipeline to satisfy the deployment contract
3. **Admission Policies** — runtime enforcement of security and compliance rules (not human review)
4. **Namespace Bootstrap API** — one command gives a team their isolated environment with correct RBAC, quotas, and network policy pre-applied
5. **Approved Catalog** — curated list of base images, tools, and runtime classes developers can choose from freely

---

## Summary

| | Current | Target |
|---|---|---|
| **Platform role** | Tool administrator + gatekeeper | Product team — builds rails, not traffic control |
| **Governance method** | Human approval | Automated policy enforcement |
| **Developer experience** | Request → wait → implement | Provision → build → deploy within governance guardrails |
| **Platform team bottleneck** | Yes — every change goes through platform | No — platform is in the background |
| **Scalability** | Limited by platform team headcount | Scales with developer count |

> The platform must be **a product developers use**, not a team developers wait on.