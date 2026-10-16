# Case Study — ArgoCD Deployment with `pgate`

## Problem

ArgoCD runs in our cluster, but only the platform team can deploy because there is one shared password and no real access control. Developers wait on tickets to get something deployed.

## Solution — `pgate` (PaaS Gate)

A CLI that lets developers deploy using their Git identity, without knowing the ArgoCD password. Before issuing a token, it checks policy automatically.

```
┌───────────────────────────────────────────────────────────┐
│                        pgate CLI                          │
│                                                           │
│   ┌────────────┐    ┌─────────────┐    ┌──────────────┐   │
│   │  Git OAuth  │──▶│ Policy Gate │──▶│ Issue Token  │   │
│   │  (login)    │    │             │   │ (10-min TTL) │   │
│   └────────────┘    │ ✓ Repo role  │   └──────┬───────┘   │
│                      │ ✓ CODEOWNERS │           │         │
│                      │ ✓ Scan result│           │         │
│                      │ ✓ ServiceNow │           │         │
│                      │ ✓ Env order  │           │         │
│                      └─────────────┘            │          │
│                       Any fail → denied         │         │
└─────────────────────────────────────────────────┼─────────┘
                                                  │
                                                  ▼
                                     Developer calls argocd CLI
                                     argocd app sync --auth-token <token>
```

**Key points:**
- Token lives **10 minutes** — just enough to deploy, useless if leaked
- pgate **only reads** scan results and ServiceNow CR status — it does not run scans or create CRs. Both happen on separate threads before the developer calls `pgate`:
  - Security scan runs in the developer's **CI pipeline**
  - ServiceNow CR is created and approved through the existing **change management process**
- Developer never sees the ArgoCD admin password
- Platform team is **out of the deployment path**

---

## CLI Example

**User input:**
```bash
pgate argocd login --repo myorg/payment-service --env staging
```

**Output:**
```
✅ Authenticated as: john.doe@company.com

📋 Policy checks...
  [1/5] Repo role .............. ✅ write
  [2/5] Ownership .............. ✅ CODEOWNERS
  [3/5] Security scan .......... ✅ passed
  [4/5] ServiceNow CR .......... ✅ CR-20260417-0042
  [5/5] Env promotion .......... ✅ dev → staging

🎫 Token issued (10 min) → export ARGOCD_AUTH_TOKEN=******
```

**User input:**
```bash
argocd app sync payment-service-staging --auth-token $ARGOCD_AUTH_TOKEN
```

**Output:**
```
payment-service   Synced  Healthy  deployment updated
```

**When it fails:**
```
  [3/5] Security scan .......... ❌ 2 critical CVEs
🚫 Token denied — https://scan.company.com/results/b7e4d09
```

---

## Extensibility

Same pattern works for any tool — `pgate` is a plugin system:

```
pgate <tool> login --repo <org/repo> --env <env>
  → Git OAuth → policy gate → scoped token (10 min)
```

| Tool | What the token grants |
|---|---|
| `pgate argocd login` | Deploy the app mapped to the repo |
| `pgate grafana login` | Dashboards scoped to the team |
| `pgate vault login` | Secrets within the team's path |
| `pgate harbor login` | Push/pull images in the team's project |
| `pgate k8s login` | Kubeconfig for the team's namespace |

One adapter per tool. Same auth, same policy gate, different backend.

**User input:**
```bash
pgate argocd login --repo myorg/payment-service --env staging
```
