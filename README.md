# pingone-kubernetes-provider

A Kubernetes operator that manages Ping Identity product deployments via a `PingEnvironment` custom resource plus one product CR per deployed product. The operator reconciles each environment and its product CRs into a single Helm release of the [`ping-devops`](https://helm.pingidentity.com) chart (version `0.12.2`).

---

## Overview

- Apply a `PingEnvironment` manifest for the shared settings, then one product CR per product — the operator aggregates them into one Helm release.
- Every product is optional: PingFederate, PingDirectory, PingDataConsole, PingAccess, PingAuthorize, PingAuthorizePAP, PingDataSync, and PingDirectoryProxy each have their own CR kind (see `config/samples/`).
- PingFederate and PingAccess deploy as separate admin and engine workloads.
- Resource sizing (CPU/memory) is driven by a single `tier` field: `development`, `staging`, or `production`.
- A shared `spec.ingress` block provides ingress class, annotations, and a global `enabled` flag to all components; per-component blocks only need `enabled` and a TLS secret reference.
- Hostnames are derived from `spec.domain` automatically or overridden per component.
- Server profiles use a structured `serverProfile` block and optional `serverProfileLayers` list for layered profiles.
- Container startup ordering is controlled via `container.waitFor` entries.

---

## Prerequisites

| Requirement | Version |
|---|---|
| Go | 1.22+ |
| Kubernetes | 1.26+ |
| kubectl | matching cluster version |
| Helm | 3.x (used as a Go library, not CLI) |
| Docker | any recent version |
| make | GNU make |

A `devops-secret` must exist in the target namespace before applying a `PingEnvironment`:

```bash
kubectl create secret generic devops-secret \
  --from-literal=PING_IDENTITY_ACCEPT_EULA=YES \
  --from-literal=PING_IDENTITY_DEVOPS_USER=<your-devops-user> \
  --from-literal=PING_IDENTITY_DEVOPS_KEY=<your-devops-key> \
  -n <target-namespace>
```

---

## Quick start

### 1. Install CRDs

```bash
make install
```

### 2. Deploy the operator

**Docker Desktop:**
```bash
make docker-desktop
```

**Any cluster (push image first):**
```bash
make docker-build docker-push IMG=<registry>/pingone-operator:latest
make deploy IMG=<registry>/pingone-operator:latest
```

### 2a. Deploy with Helm (local chart)

```bash
helm upgrade --install pingone-operator ./charts/pingone-operator \
  --namespace pingone-system \
  --create-namespace \
  --set image.repository=<registry>/pingone-operator \
  --set image.tag=<tag>
```

### 2b. Package and deploy via OCI chart registry

```bash
# Authenticate to your OCI registry first (example: GHCR)
echo <token> | helm registry login ghcr.io -u <user> --password-stdin

# Package chart and push to OCI repo
make helm-oci-push HELM_OCI_REPO=oci://ghcr.io/<org>/charts HELM_VERSION=0.1.0

# Install/upgrade from OCI
helm upgrade --install pingone-operator oci://ghcr.io/<org>/charts/pingone-operator \
  --version 0.1.0 \
  --namespace pingone-system \
  --create-namespace \
  --set image.repository=<registry>/pingone-operator \
  --set image.tag=<tag>
```

If you want `make` wrappers for install from OCI:

```bash
make helm-oci-install \
  HELM_OCI_REPO=oci://ghcr.io/<org>/charts \
  HELM_VERSION=0.1.0
```

### 3. Apply an environment

Use the getting-started example (edit credentials first):

```bash
make example-getting-started
```

Or apply the minimal sample CR:

```bash
kubectl apply -f config/samples/pingone_v1alpha1_pingenvironment.yaml
```

Or use your own manifest — see [Configuration](#configuration) below.

### 4. Check status

```bash
kubectl get pingenvironments -A
kubectl get pods -n <target-namespace>
```

---

## Manifests

A `PingEnvironment` defines the shared settings; each product is deployed by its own CR referencing the environment via `spec.environmentRef`:

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingEnvironment
metadata:
  name: myorg-dev
  namespace: pingone
spec:
  tenantId: myorg-dev           # prefix for the Helm release name (<tenantId>-ping)
  tier: development             # development | staging | production
  domain: dev.myorg.example.com # base domain; component hostnames derived automatically

  # Shared ingress — inherited by all product ingress blocks unless overridden
  ingress:
    enabled: true               # globally enable ingress for all components
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
---
apiVersion: pingone.io/v1alpha1
kind: PingFederate
metadata:
  name: myorg-dev-pf
  namespace: pingone
spec:
  environmentRef: myorg-dev
  version: "13.0.2-edge"
  engineIngress:
    enabled: true
    tlsSecretRef: pf-tls        # hostname: pf.dev.myorg.example.com
  adminIngress:
    enabled: true
    tlsSecretRef: pf-admin-tls  # hostname: pf-admin.dev.myorg.example.com
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      branch: main
      path: pingfederate
---
apiVersion: pingone.io/v1alpha1
kind: PingDirectory
metadata:
  name: myorg-dev-pd
  namespace: pingone
spec:
  environmentRef: myorg-dev
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      branch: main
      path: pingdirectory
    userBaseDN: "dc=myorg,dc=com"
```

The same pattern applies to `PingAccess`, `PingAuthorize`, `PingAuthorizePAP`, `PingDataSync`, `PingDirectoryProxy`, and `PingDataConsole` — see `config/samples/` for a starter manifest per kind and [CONFIGURATION.md](CONFIGURATION.md) for the full field reference. Deleting a product CR removes that product from the Helm release.

---

## Configuration

See **[CONFIGURATION.md](CONFIGURATION.md)** for complete documentation including:

- All config fields and their container env var mappings for each product
- Resource sizing per tier
- Ingress configuration (global and per-component)
- Server profile and layered profile configuration
- Container `waitFor` dependency ordering
- `valuesOverride` escape hatch for raw Helm values
- Status fields

---

## Hostname derivation

When `spec.domain` is set, hostnames are derived automatically:

| Component | Default hostname |
|---|---|
| PingFederate engine | `pf.<domain>` |
| PingFederate admin | `pf-admin.<domain>` |
| PingDataConsole | `pd-console.<domain>` |
| PingAccess admin | `pa-admin.<domain>` |
| PingAccess engine | `pa.<domain>` |
| PingAuthorize | `paz.<domain>` |
| PingAuthorizePAP | `paz-pap.<domain>` |
| PingDataSync | `pds.<domain>` |
| PingDirectoryProxy | `pdp.<domain>` |

Override any hostname by setting `hostname` inside the component's ingress block.

---

## Tier resource sizing

| Tier | PingFederate / PingAccess CPU/Mem | PingDirectory / PingAuthorize / PingDataSync / PingDirectoryProxy CPU/Mem |
|---|---|---|
| `development` | 500m / 512Mi | 500m / 1Gi |
| `staging` | 1 / 1Gi | 1 / 2Gi |
| `production` | 2 / 2Gi | 2 / 4Gi |

PingDataConsole and PingAuthorizePAP are lightweight UI/API components — no tier-based resource sizing is applied.

---

## Server profiles

All products support structured server profiles and optional layered profiles:

```yaml
config:
  serverProfile:
    url: https://github.com/myorg/ping-profiles.git
    branch: main      # optional
    path: pingfederate
    parent: baseline  # optional: name of a layer this profile chains to

  serverProfileLayers:
    - name: baseline
      url: https://github.com/myorg/ping-profiles.git
      path: baseline/pingfederate
      branch: main    # optional
```

The `serverProfile` block maps to `SERVER_PROFILE_URL`, `SERVER_PROFILE_BRANCH`, `SERVER_PROFILE_PATH`, and `SERVER_PROFILE_PARENT` container env vars.

Each entry in `serverProfileLayers` maps to `SERVER_PROFILE_<NAME>_URL`, `SERVER_PROFILE_<NAME>_BRANCH`, `SERVER_PROFILE_<NAME>_PATH`, and `SERVER_PROFILE_<NAME>_PARENT` env vars, where `<NAME>` is the `name` field uppercased.

See [Ping Identity layered profiles documentation](https://developer.pingidentity.com/devops/how-to/profilesLayered.html) for details on chaining profiles.

---

## Container wait-for

Products can wait for other containers to become ready before starting:

```yaml
container:
  waitFor:
    - application: pingDirectory
      service: ldaps
      timeoutSeconds: 300
```

The `application` field accepts logical names (case-insensitive):

| Logical name | Resolves to |
|---|---|
| `pingDirectory` | `pingdirectory` |
| `pingFederate` / `pingFederateEngine` | `pingfederate-engine` |
| `pingFederateAdmin` | `pingfederate-admin` |
| `pingAccess` / `pingAccessEngine` | `pingaccess-engine` |
| `pingAccessAdmin` | `pingaccess-admin` |
| `pingAuthorize` | `pingauthorize` |
| `pingAuthorizePAP` | `pingauthorizepap` |
| `pingDataConsole` | `pingdataconsole` |
| `pingDataSync` | `pingdatasync` |
| `pingDirectoryProxy` | `pingdirectoryproxy` |

---

## Examples

| Example | Description |
|---|---|
| [examples/getting-started](examples/getting-started/) | PingFederate on Docker Desktop with nginx ingress, cert-manager self-signed TLS, and the Ping Identity getting-started server profile |

## Development

```bash
make tools        # download controller-gen, kustomize, golangci-lint, envtest
make all          # generate + fmt + vet + build
make test         # run tests with envtest
make lint         # run golangci-lint
make run          # run operator locally against active kubeconfig
```

Full target list:

```bash
make help
```

---

## Project structure

```
api/v1alpha1/               CRD types and DeepCopy
config/
  crd/                      Generated CRD manifests
  rbac/                     RBAC roles and bindings
  manager/                  Operator Deployment manifest
  default/                  Kustomize overlay (namespace, namePrefix)
  samples/                  Example PingEnvironment CR
controllers/                Reconciler (pingenvironment_controller.go)
internal/helm/
  client.go                 Helm action.Configuration setup
  defaults.go               Ping Identity container env var defaults
  release.go                BuildPingValues, InstallOrUpgrade, DownloadChart
CONFIGURATION.md            Full field reference
```

---

## License

Apache 2.0
