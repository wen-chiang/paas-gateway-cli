You are helping a developer deploy this application using `pgate`.
`pgate` enforces GitHub identity, policy checks, and OIDC login before proxying any ArgoCD command.
Never run `argocd` directly — always use `./pgate argocd run`.

---

## Before running any pgate command

**1. Detect current branch and repo URL from git:**
```bash
git rev-parse --abbrev-ref HEAD
git remote get-url origin
```
- Use the branch value as `--branch` in every pgate command, and as `PGATE_BRANCH` when rendering the template. Never hardcode.
- Strip `.git` suffix and extract `org/repo` from the remote URL for `--repo`. The full remote URL (with `.git`) is `PGATE_REPO_URL` used only in template rendering.

**2. Check GitHub credentials:**
```bash
echo "USER=${PGATE_GITHUB_USER}  TOKEN=${PGATE_GITHUB_TOKEN:+set}"
```

If either is missing, stop and tell the user:
> Please set your GitHub credentials so pgate can run non-interactively:
>
> ```bash
> export PGATE_GITHUB_USER=your-username
> export PGATE_GITHUB_TOKEN=your-personal-access-token
> ```
>
> Add these to your `~/.zshrc` or `~/.bashrc` so they persist.
> Generate a token at: https://github.sampleOrg.com/settings/tokens (scopes: `repo`)

Once set, re-run the `/pgate` command.

The browser OIDC step (Keycloak) still requires the user to log in manually —
pgate opens the browser automatically. Tell the user to complete the login and return to the terminal.

---

## Environment → app name mapping

| Environment | ArgoCD app name |
|---|---|
| `dev` | `<repo-name>-dev` |
| `trial` | `<repo-name>-trial` |
| `prod` | `<repo-name>-prod` |

`<repo-name>` is the repository name portion of the remote URL (e.g. `otel-op-springboot-app`).

---

## When the user says "/pgate init" or "/pgate init <env>"

Initialize ArgoCD applications by rendering the template and creating them.
If no env is specified, create all three environments in order: dev → trial → prod.

**Step 1 — Check `argocd-application.tmpl.yaml` exists:**
```bash
ls argocd-application.tmpl.yaml
```
If missing, stop and tell the user:
> `argocd-application.tmpl.yaml` not found. Copy it from the pgate skill-package into the repo root.

**Step 2 — Search the repo for values before asking anything.**

Read as many files as you can find to extract values. Do NOT ask the user for anything you can discover yourself.

Search in this order — stop looking for a value once it is found:

| Value | Where to look |
|---|---|
| `PGATE_NAMESPACE` | existing `argocd-application-*.yaml` (`spec.destination.namespace`), `.pgate.yaml`, `values*.yaml` (`namespace:`) |
| `PGATE_PROJECT` | existing `argocd-application-*.yaml` (`spec.project`), `.pgate.yaml` |
| `PGATE_CLUSTER` | existing `argocd-application-*.yaml` (`spec.destination.name`), default `in-cluster` |
| `PGATE_CHART_PATH` | existing `argocd-application-*.yaml` (`spec.source.path`), search for `Chart.yaml` with `find . -maxdepth 4 -name Chart.yaml` and use its parent directory |
| `PGATE_HELM_VALUES` | existing `argocd-application-*.yaml` (`spec.source.helm.valueFiles[0]`), look for `values-<env>.yaml` then `values.yaml` |

Also search:
- `target-application.yaml` — may contain all fields already filled in
- Any `*.yaml` in the repo root that looks like an ArgoCD Application manifest (`kind: Application`)
- `.pgate.yaml` for any `namespace`, `project`, or `app_name` keys

**Step 3 — Show what was found, then ask only for what is still missing.**

Present a summary table of resolved vs. missing values, for example:
```
PGATE_NAMESPACE  → dev             (from argocd-application-dev.yaml)
PGATE_PROJECT    → demo            (from target-application.yaml)
PGATE_CLUSTER    → in-cluster      (default)
PGATE_CHART_PATH → charts/my-app   (from Chart.yaml at charts/my-app/Chart.yaml)
PGATE_HELM_VALUES→ values.yaml     (found in repo root)
```
Only ask the user to supply values that are still blank after the search.

**Step 4 — Render the template directly (no envsubst needed).**

Read `argocd-application.tmpl.yaml`, substitute every `${PGATE_*}` placeholder with the resolved value, and write the rendered file as `argocd-application-<env>.yaml`. Show the rendered YAML to the user for confirmation before applying.

**Step 5 — Check first, then create:**
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app get <app-name>
```
If exit code is 0, skip creation and report it already exists.

Otherwise create from the rendered file:
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app create -f argocd-application-<env>.yaml
```

Run for each environment the user requested. Report which apps were created and which already existed.

---

## When the user says "/pgate deploy to <env>"

Run these steps in order using the Bash tool:

**1. Check if app exists — if not, tell the user to run `/pgate init <env>` first**
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app get <app-name>
```

If exit code is non-zero, stop and tell the user:
> App not found. Run `/pgate init <env>` first to register it in ArgoCD.

**2. Sync (deploy)**
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app sync <app-name>
```

**3. Wait until healthy**
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app wait <app-name> --health
```

**4. Confirm final status**
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app get <app-name>
```

---

## When the user says "/pgate status <env>"

```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app get <app-name>
```

---

## When the user says "/pgate logs <env>"

```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app logs <app-name> --follow
```

---

## When the user says "/pgate rollback <env>"

First show history so the user can pick a revision:
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app history <app-name>
```

Ask the user which revision, then rollback:
```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  -- app rollback <app-name> <revision>
```

---

## When the user says "/pgate dry-run <env>"

```bash
./pgate argocd run \
  --repo <repo> \
  --env <env> \
  --branch <branch> \
  --dry-run \
  -- app sync <app-name>
```

---

## Promotion order

`dev` → `trial` → `prod`

pgate enforces this automatically. Warn the user if they try to skip an environment.

---

## Notes for Claude Code

- `<repo>`, `<branch>`, `PGATE_REPO_URL` are always resolved from git — never hardcoded
- `<app-name>` is always `<repo-name>-<env>` derived from the git remote
- Ask for namespace, project, cluster, chart path, and values file before init — these vary per team
- `pgate` opens a browser for Keycloak OIDC login — tell the user to complete login in their browser and return to the terminal
- After login the terminal shows `✅ Browser authentication complete.` and continues automatically
- Token identity is printed — confirm the user's groups include `/G-dev` or `/G-admin`
- If port 7075 is already in use, ask the user to stop the conflicting process and retry
- For prod deployments, `--crq <CHG-number>` is required (e.g. `--crq CHG0012345`)
