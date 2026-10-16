# paas-gateway-cli

A CLI gateway that enforces policy checks and proxies ArgoCD commands — using the user's own OIDC identity, not a shared service account.

---

## How it works

```
pgate argocd run
  │
  ├─ 1. GitHub login      — verify your identity (username + password)
  ├─ 2. Policy checks     — repo role, CODEOWNERS, security scan, ServiceNow CR, env order
  ├─ 3. ArgoCD OIDC login — browser opens → you authenticate → id_token returned
  └─ 4. Proxy command     — argocd CLI called with --server + --auth-token (your id_token)
```

The token is **your own OIDC identity** — ArgoCD audit logs show your username, not a shared admin account. You can also log into the ArgoCD GUI with the same identity at any time.

---

## Prerequisites

- `argocd` CLI installed and in `PATH` (only needed for `pgate argocd run`)
- Access to a GitHub instance (configured in `config.yaml`)
- ArgoCD configured with OIDC (Dex or external provider) — see [ArgoCD OIDC setup](#argocd-oidc-setup)

---

## Commands

```
pgate argocd [command]

  run     Run any ArgoCD command with policy enforcement — token never exposed
  login   Authenticate via GitHub and get a scoped ArgoCD deploy token (debug/dry-run)
```

---

## argocd run — secure proxy (recommended)

```bash
pgate argocd run --repo <org/repo> --env <env> [--branch <branch>] -- <argocd args>
```

`pgate` authenticates via GitHub, runs policy checks, resolves the expected app name from `--repo` + `--env`, validates it against the command, then proxies to the `argocd` CLI with `--server` and `--auth-token` injected automatically. The token is **never printed**.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--repo` | ✅ | GitHub repo in `org/name` format |
| `--env` | ✅ | Target environment (`dev`, `staging`, `prod`) |
| `--branch` | | Branch to read `.pgate.yaml` and `CODEOWNERS` from (default: repo default branch) |
| `--dry-run` | | Run policy checks and validate app name — do not execute command |

**Examples:**

```bash
# ── Sync ──────────────────────────────────────────────────────────────────────

# sync the app (default branch)
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  -- app sync otel-op-springboot-app-dev

# sync the app — read .pgate.yaml / CODEOWNERS from the develop branch
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app sync otel-op-springboot-app-dev

# sync and wait until the app is healthy (combine flags after --)
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app sync otel-op-springboot-app-dev --force --prune

# ── Status & inspection ───────────────────────────────────────────────────────

# get app status
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app get otel-op-springboot-app-dev

# wait until healthy
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app wait otel-op-springboot-app-dev --health

# view deployment history
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app history otel-op-springboot-app-dev

# stream live logs from the running pods
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app logs otel-op-springboot-app-dev --follow

# list all apps (no app name — passes through policy gate, no name check)
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  -- app list

# ── Rollback ──────────────────────────────────────────────────────────────────

# roll back to the previous revision (history ID from app history)
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  -- app rollback otel-op-springboot-app-dev 3

# ── Promote: dev → staging → prod ────────────────────────────────────────────

# deploy to staging after dev is verified
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env staging \
  --branch develop \
  -- app sync otel-op-springboot-app-staging

# deploy to prod after staging is verified
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env prod \
  --branch develop \
  -- app sync otel-op-springboot-app-prod

# ── Dry-run ───────────────────────────────────────────────────────────────────

# validate policy + app name without executing the command
pgate argocd run \
  --repo repoOrg/otel-op-springboot-app \
  --env dev \
  --branch develop \
  --dry-run \
  -- app sync otel-op-springboot-app-dev
```

**App name enforcement:**

If the app name in the command doesn't match what `pgate` derived from `--repo` + `--env`, the command is denied — even with a valid token.

```
🚫 app name mismatch
   command targets : payment-service-dev
   gated for       : otel-op-springboot-app-dev (repo=repoOrg/otel-op-springboot-app env=dev)
```

Supported app-targeting verbs: `sync`, `get`, `wait`, `delete`, `history`, `rollback`, `set`, `unset`, `patch`, `terminate-op`, `logs`.
List/non-targeting commands (e.g. `app list`, `cluster list`) pass through without an app name check.

---

## argocd login — debug / dry-run

```bash
pgate argocd login --repo <org/repo> --env <env> [--branch <branch>] [--dry-run]
```

Runs the full auth + policy flow and prints the token for manual inspection. Use `--dry-run` to see policy results without issuing a token.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--repo` | ✅ | GitHub repo in `org/name` format |
| `--env` | ✅ | Target environment (`dev`, `staging`, `prod`) |
| `--branch` | | Branch to read `.pgate.yaml` and `CODEOWNERS` from (default: repo default branch) |
| `--dry-run` | | Run policy checks only — do not issue a token |

**Example — dry run, dev, develop branch:**

```
$ pgate argocd login \
    --repo repoOrg/otel-op-springboot-app \
    --env dev \
    --branch develop \
    --dry-run

🔐 GitHub login (https://github.sampleOrg.com)
   Username: sampleOrg.
   Password:
✅ Authenticated as: sampleOrg.

📋 Policy checks...
  [1/5] Repo role      ✅ admin
  [2/5] Ownership      ✅ no CODEOWNERS (skipped)
  [3/5] Security scan  ✅ passed [stub]
  [4/5] ServiceNow CR  ✅ CR-STUB-0000 [stub]
  [5/5] Env promotion  ✅ dev (first env)

🌐 Opening browser for ArgoCD OIDC login...
   If the browser does not open automatically, visit:
   https://argocd.sampleOrg.local/api/dex/auth?client_id=argo-cd&...

✅ Browser authentication complete.

✅ Dry run — policy passed, token issued.
   ArgoCD app : otel-op-springboot-app-dev
   Server     : https://argocd.sampleOrg.local
   Token      : eyJhbGciOiJIUzI1...

   Next step  : pgate argocd run --repo repoOrg/otel-op-springboot-app --env dev --branch develop -- app sync otel-op-springboot-app-dev

   (dry-run: no deployment triggered)
```

**Example — issue a real token (for manual argocd CLI use):**

```
$ pgate argocd login \
    --repo repoOrg/otel-op-springboot-app \
    --env dev \
    --branch develop

🔐 GitHub login (https://github.sampleOrg.com)
   Username: sampleOrg.
   Password:
✅ Authenticated as: sampleOrg.

📋 Policy checks...
  [1/5] Repo role      ✅ admin
  [2/5] Ownership      ✅ no CODEOWNERS (skipped)
  [3/5] Security scan  ✅ passed [stub]
  [4/5] ServiceNow CR  ✅ CR-STUB-0000 [stub]
  [5/5] Env promotion  ✅ dev (first env)

🌐 Opening browser for ArgoCD OIDC login...
   If the browser does not open automatically, visit:
   https://argocd.sampleOrg.local/api/dex/auth?client_id=argo-cd&...

✅ Browser authentication complete.

🎫 Token issued (OIDC session)
   Token : eyJhbGciOiJIUzI1...

   Next  : pgate argocd run --repo repoOrg/otel-op-springboot-app --env dev --branch develop -- app sync otel-op-springboot-app-dev
```

---

## How `--branch` works

`--branch` controls which branch `pgate` reads **repo-side config files** from:

| File | Purpose |
|---|---|
| `.pgate.yaml` | Per-repo app name overrides and env mappings |
| `CODEOWNERS` | Ownership check — must list `@sampleOrg.` |

It does **not** affect which branch ArgoCD deploys from — that is configured in the ArgoCD Application manifest itself.

```
without --branch  →  GitHub reads from the repo default branch (usually main/master)
--branch develop  →  GitHub reads .pgate.yaml and CODEOWNERS from develop
```

---

## App name resolution

`pgate` derives the ArgoCD app name from `--repo` + `--env` using this priority order:

| Priority | Source | Example result |
|---|---|---|
| 1 | `.pgate.yaml` → `env_apps.<env>` | custom name per env |
| 2 | `.pgate.yaml` → `app_name` + `-` + env | `otel-op-springboot-app-dev` |
| 3 | `config.yaml` → `repos.<repo>.env_apps.<env>` | static fallback |
| 4 | Last segment of `--repo` + `-` + env | `otel-op-springboot-app-dev` |

To override the default name, commit a `.pgate.yaml` to your repo:

```yaml
# .pgate.yaml (in repoOrg/otel-op-springboot-app)
app_name: otel-op-springboot-app
env_apps:
  dev:     otel-op-springboot-app-dev
  staging: otel-op-springboot-app-staging
  prod:    otel-op-springboot-app-prod
```

---

## ArgoCD OIDC setup

ArgoCD must be configured with an OIDC provider (Dex is built-in). pgate reads the OIDC settings automatically from `/api/v1/settings` — no extra pgate config is needed.

**Using Dex with GitHub Enterprise (github.sampleOrg.com):**

Add this to your `argocd-cm` ConfigMap:

```yaml
# kubectl -n argocd edit configmap argocd-cm
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  url: https://argocd.sampleOrg.local

  oidc.config: |
    name: GitHub
    issuer: https://argocd.sampleOrg.local/api/dex
    clientID: argo-cd
    clientSecret: $oidc.github.clientSecret
    requestedScopes:
      - openid
      - profile
      - email
      - groups

  dex.config: |
    connectors:
      - type: github
        id: github
        name: GitHub Enterprise
        config:
          hostName: github.sampleOrg.com
          clientID: <your-github-oauth-app-client-id>
          clientSecret: $dex.github.clientSecret
          orgs:
            - name: <your-github-org>
```

**What the user sees:**

When `pgate` passes policy checks, it opens a browser to the ArgoCD Dex login page. The user authenticates with their GitHub credentials once and is redirected back. The resulting `id_token` is used as the ArgoCD auth token — the same identity that works for the ArgoCD GUI.

**Verify OIDC is configured:**

```bash
curl -k https://argocd.sampleOrg.local/api/v1/settings | jq '.oidcConfig'
# Should return { "issuer": "...", "clientID": "argo-cd", ... }
# If null — OIDC is not configured yet
```

---

## Configuration

Config is loaded from (in order):

1. `~/.config/pgate/config.yaml` — user-level, applies globally
2. `./config.yaml` — project root, for local development

See [config.yaml](config.yaml) for a full annotated example.
