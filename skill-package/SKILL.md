# pgate Deploy Skill

This project uses `pgate` for all ArgoCD deployments.
`pgate` enforces GitHub identity, policy checks, and OIDC login before proxying any ArgoCD command.

> **Never run `argocd` directly.** Always use `./pgate argocd run` so policy gates are enforced.

---

## Setup (copy this skill-package into your repo)

1. Copy `skill-package/` contents into your repo root:
   ```
   argocd-application.tmpl.yaml   ← ArgoCD app template
   SKILL.md                        ← this file
   .claude/commands/pgate.md       ← Claude Code /pgate skill
   ```
2. Place the `pgate` binary in your repo root (get it from the pgate release or your platform team).
3. Make it executable: `chmod +x pgate`

---

## Prerequisites

- `argocd` CLI installed and in `PATH`
- `./pgate` binary present in the project root
- Access to GitHub Enterprise (`github.sampleOrg.com`) with write permission on this repo
- A browser available for Keycloak OIDC login
- GitHub credentials set in your shell:
  ```bash
  export PGATE_GITHUB_USER=your-username
  export PGATE_GITHUB_TOKEN=your-personal-access-token
  ```

---

## Claude Code slash commands

With this skill installed, Claude Code understands:

| Command | What it does |
|---|---|
| `/pgate init <env>` | First-time setup: renders template and creates the ArgoCD app |
| `/pgate deploy to <env>` | Sync and wait until healthy |
| `/pgate status <env>` | Show current app health and sync status |
| `/pgate logs <env>` | Stream live logs |
| `/pgate rollback <env>` | Show history and roll back to a revision |
| `/pgate dry-run <env>` | Validate policy without deploying |

Claude detects `--repo` and `--branch` from your local git automatically.

---

## Workflow

### Step 1 — First time only: init the app in ArgoCD

Run `/pgate init <env>` in Claude Code. It will:
1. Read `argocd-application.tmpl.yaml`
2. Search your repo for namespace, project, cluster, chart path, and values file
3. Ask only for values it could not find
4. Render `argocd-application-<env>.yaml` and show it to you before applying
5. Create the app in ArgoCD

Or run manually — replace `<branch>` with your current branch:

```bash
# Render the template first (fill in your values):
export PGATE_APP_NAME=<repo-name>-dev
export PGATE_NAMESPACE=dev
export PGATE_REPO_URL=$(git remote get-url origin)
export PGATE_BRANCH=$(git rev-parse --abbrev-ref HEAD)
export PGATE_PROJECT=demo
export PGATE_CLUSTER=in-cluster
export PGATE_CHART_PATH=charts/my-app
export PGATE_HELM_VALUES=values.yaml
envsubst < argocd-application.tmpl.yaml > argocd-application-dev.yaml

# Create the app
./pgate argocd run \
  --repo <org/repo> \
  --env dev \
  --branch <branch> \
  -- app create -f argocd-application-dev.yaml
```

To update an existing app definition, re-run with `--upsert`:
```bash
./pgate argocd run \
  --repo <org/repo> \
  --env dev \
  --branch <branch> \
  -- app create --upsert -f argocd-application-dev.yaml
```

---

### Step 2 — Deploy (sync)

```bash
./pgate argocd run \
  --repo <org/repo> \
  --env dev \
  --branch <branch> \
  -- app sync <app-name>
```

---

### Step 3 — Wait until healthy

```bash
./pgate argocd run \
  --repo <org/repo> \
  --env dev \
  --branch <branch> \
  -- app wait <app-name> --health
```

---

## Other useful commands

### Check app status
```bash
./pgate argocd run --repo <org/repo> --env dev --branch <branch> -- app get <app-name>
```

### View live logs
```bash
./pgate argocd run --repo <org/repo> --env dev --branch <branch> -- app logs <app-name> --follow
```

### View deployment history
```bash
./pgate argocd run --repo <org/repo> --env dev --branch <branch> -- app history <app-name>
```

### Rollback to a previous revision
```bash
# Get revision number from app history first, then:
./pgate argocd run --repo <org/repo> --env dev --branch <branch> -- app rollback <app-name> <revision>
```

### Dry-run — validate policy without deploying
```bash
./pgate argocd run --repo <org/repo> --env dev --branch <branch> --dry-run -- app sync <app-name>
```

---

## Promotion order

```
dev → trial → prod
```

pgate enforces this automatically. Deploying to `trial` is blocked until `dev` passes policy.
Deploying to `prod` requires a CRQ number: add `--crq CHG0012345`.

---

## How pgate works

```
./pgate argocd run
  │
  ├─ 1. GitHub login        verify identity via github.sampleOrg.com
  ├─ 2. Policy checks       repo role · CODEOWNERS · security scan · ServiceNow CRQ · env order
  ├─ 3. ArgoCD OIDC login   browser opens → Keycloak → id_token returned
  └─ 4. Proxy command       argocd CLI called with --server + --auth-token (your identity)
```

The ArgoCD token is your own OIDC identity — never a shared service account.
ArgoCD audit logs show your username for every action.
