# PingOne Operator — Configuration Guide

The operator manages Ping Identity products in Kubernetes via a `PingEnvironment` custom resource plus one product CR per deployed product (`PingFederate`, `PingDirectory`, `PingAccess`, `PingAuthorize`, `PingAuthorizePAP`, `PingDataSync`, `PingDirectoryProxy`, `PingDataConsole`). Each environment maps to one Helm release of the `ping-devops` chart (version `0.12.2` from `https://helm.pingidentity.com`).

---

## How it works

1. You apply a `PingEnvironment` manifest defining the shared settings: tenant ID, tier, domain, global ingress, and global env vars.
2. You apply one product CR for each product you want deployed. Each product CR points at its environment via `spec.environmentRef`.
3. The operator aggregates the environment and all product CRs that reference it into a single `ping-devops` Helm values map.
4. A single Helm release named `<tenantId>-ping` is installed or upgraded in the target namespace. Deleting a product CR removes that product from the release on the next reconcile.
5. All Ping Identity container env vars are driven from CRD fields; use `valuesOverride` on any product CR as an escape hatch for anything not exposed by the CRD.

Every product is optional — deploy only the CRs you need. When several CRs of the same kind reference one environment, the first by name wins and a warning is logged.

### Prerequisites

A `devops-secret` must exist in the target namespace before the operator deploys:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: devops-secret
  namespace: <target-namespace>
type: Opaque
data:
  PING_IDENTITY_ACCEPT_EULA: <base64 of "YES">
  PING_IDENTITY_DEVOPS_USER: <base64 of your devops user email>
  PING_IDENTITY_DEVOPS_KEY:  <base64 of your devops key>
```

The operator injects this secret into every product container via `global.envFrom.secretRef`.

---

## PingEnvironment

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.tenantId` | string | Yes | Unique identifier for this environment. Used as the Helm release name prefix (`<tenantId>-ping`). Must be a DNS-1123 label fragment (max 30 chars). |
| `spec.tier` | string | Yes | One of `development`, `staging`, `production`. Controls CPU/memory resource requests per product. |
| `spec.domain` | string | No | Base domain for automatic hostname derivation (e.g. `dev.myorg.example.com`). See [Hostname derivation](#hostname-derivation). |
| `spec.targetNamespace` | string | No | Namespace to deploy the Helm release into. Defaults to the CR's own namespace. |
| `spec.ingress` | object | No | Shared ingress settings inherited by all product ingress blocks. See [spec.ingress](#specingress-global). |
| `spec.services` | object | No | Shared Kubernetes Service settings (`annotations`) inherited by all products. Per-product `service.annotations` win on conflict. |
| `spec.envs` | map | No | Env vars injected into every product container (`global.envs`). |
| `spec.vault` | object | No | HashiCorp Vault Agent injection settings for all products (`global.vault`). Raw chart values. |
| `spec.securityContext` | object | No | Pod-level securityContext for all product workloads. Per-product `container.securityContext` overrides. |
| `spec.containerSecurityContext` | object | No | Container-level securityContext for all product workloads. Per-product `container.containerSecurityContext` overrides. |
| `spec.resources` | object | No | Default CPU/memory requests/limits for all product containers. Per-product `container.resources` overrides. |
| `spec.volumes` | map | No | Named pod-level volumes available to all products. Products opt in via `container.includeVolumes`. |
| `spec.includeVolumes` | list | No | Volume names (from `spec.volumes`) mounted into every product workload (`global.includeVolumes`). |
| `spec.pingDataConsole` | object | No | **Deprecated** — inline PingDataConsole settings. Create a [PingDataConsole](#pingdataconsole) CR instead; a CR referencing this environment takes precedence. |

### Resource sizing by tier

| Tier | PingFederate / PingAccess CPU/Mem | PingDirectory / PingAuthorize / PingDataSync / PingDirectoryProxy CPU/Mem |
|---|---|---|
| `development` | 500m / 512Mi | 500m / 1Gi |
| `staging` | 1 / 1Gi | 1 / 2Gi |
| `production` | 2 / 2Gi | 2 / 4Gi |

PingDataConsole and PingAuthorizePAP are lightweight UI/API components and do not receive tier-based resource sizing.

---

## spec.ingress (global)

The global ingress block on PingEnvironment provides defaults inherited by all product ingress blocks. Per-component blocks take precedence when set.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Globally enables ingress for all components. Each component can override this with its own `enabled` field. |
| `className` | string | — | `ingressClassName` (e.g. `nginx`, `traefik`, `alb`). |
| `tlsSecretRef` | string | — | Default TLS Secret name applied when a component does not set its own. |
| `annotations` | map | — | Annotations merged into all component Ingress resources. Component-level annotations are merged on top. |

Example:
```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
```

---

## Hostname derivation

When `spec.domain` is set on the environment, hostnames for each product are derived as `<prefix>.<domain>`:

| Component | Prefix | Example (`domain: dev.example.com`) |
|---|---|---|
| PingFederate engine | `pf` | `pf.dev.example.com` |
| PingFederate admin | `pf-admin` | `pf-admin.dev.example.com` |
| PingDataConsole | `pd-console` | `pd-console.dev.example.com` |
| PingAccess admin | `pa-admin` | `pa-admin.dev.example.com` |
| PingAccess engine | `pa` | `pa.dev.example.com` |
| PingAuthorize | `paz` | `paz.dev.example.com` |
| PingAuthorizePAP | `paz-pap` | `paz-pap.dev.example.com` |
| PingDataSync | `pds` | `pds.dev.example.com` |
| PingDirectoryProxy | `pdp` | `pdp.dev.example.com` |

Override any hostname by setting `hostname` inside the component's ingress block.

---

## Server profiles

All products support structured server profiles and optional layered profiles.

### Base profile

The `serverProfile` block maps to these container env vars:

| CRD field | Env var |
|---|---|
| `serverProfile.url` | `SERVER_PROFILE_URL` |
| `serverProfile.branch` | `SERVER_PROFILE_BRANCH` |
| `serverProfile.path` | `SERVER_PROFILE_PATH` |
| `serverProfile.parent` | `SERVER_PROFILE_PARENT` |

### Layered profiles

The `serverProfileLayers` list supports arbitrary-depth profile chaining. Each entry maps to `SERVER_PROFILE_<NAME>_*` env vars where `<NAME>` is the `name` field uppercased.

| CRD field | Env var |
|---|---|
| `serverProfileLayers[].url` | `SERVER_PROFILE_<NAME>_URL` |
| `serverProfileLayers[].branch` | `SERVER_PROFILE_<NAME>_BRANCH` |
| `serverProfileLayers[].path` | `SERVER_PROFILE_<NAME>_PATH` |
| `serverProfileLayers[].parent` | `SERVER_PROFILE_<NAME>_PARENT` |

Example with layered profiles:
```yaml
config:
  serverProfile:
    url: https://github.com/myorg/ping-profiles.git
    branch: main
    path: paz-pap-integration/pingauthorize
    parent: baseline
  serverProfileLayers:
    - name: baseline
      url: https://github.com/myorg/ping-profiles.git
      branch: main
      path: baseline/pingauthorize
```

This emits:
- `SERVER_PROFILE_URL=https://github.com/myorg/ping-profiles.git`
- `SERVER_PROFILE_BRANCH=main`
- `SERVER_PROFILE_PATH=paz-pap-integration/pingauthorize`
- `SERVER_PROFILE_PARENT=baseline`
- `SERVER_PROFILE_BASELINE_URL=https://github.com/myorg/ping-profiles.git`
- `SERVER_PROFILE_BASELINE_BRANCH=main`
- `SERVER_PROFILE_BASELINE_PATH=baseline/pingauthorize`

See [Ping Identity layered profiles](https://developer.pingidentity.com/devops/how-to/profilesLayered.html) for details.

---

## Container settings

All products accept a `spec.container` block:

| Field | Type | Description |
|---|---|---|
| `waitFor` | list | Startup dependency probes. See below. |
| `resources` | object | CPU/memory requests/limits. Overrides tier defaults and `PingEnvironment.spec.resources`. |
| `securityContext` | object | Pod-level securityContext override for this product. |
| `containerSecurityContext` | object | Container-level securityContext override for this product. |
| `includeVolumes` | list | Volume names from `PingEnvironment.spec.volumes` to mount into this product's pod. |
| `volumes` | list | Additional pod-level volumes (standard Kubernetes volume specs). |
| `volumeMounts` | list | Additional container volumeMounts (standard Kubernetes volumeMount specs). |

### Container wait-for

Each `waitFor` entry causes the container to probe a dependency before starting:

```yaml
container:
  waitFor:
    - application: pingDirectory
      service: ldaps
      timeoutSeconds: 300
```

| Field | Type | Description |
|---|---|---|
| `application` | string | Logical name of the dependency (see table below). Case-insensitive. |
| `service` | string | Service port name to probe (e.g. `ldaps`, `https`). |
| `timeoutSeconds` | int | How long to wait before giving up. |

**Application name mapping:**

| Logical name | Resolves to Helm sub-chart |
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

## PingFederate

Deployed by creating a `PingFederate` CR. It renders as two Deployment workloads: an admin console (`pingfederate-admin`) and one or more runtime engines (`pingfederate-engine`).

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingFederate
metadata:
  name: dev-pf
spec:
  environmentRef: env-dev
  replicas: 1
  engineIngress:
    enabled: true
  adminIngress:
    enabled: true
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      path: pingfederate
```

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override (e.g. `registry.example.com/org`). |
| `version` | string | — | Image tag or full reference (e.g. `13.0.2-edge`). |
| `replicas` | int | `1` | Number of engine pods. The admin always runs as a single pod. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged last on top of all computed Helm values. |

### spec.engineIngress / adminIngress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited from environment `spec.ingress.enabled` | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `pf.<domain>` / `pf-admin.<domain>` | FQDN for this component. |
| `tlsSecretRef` | string | `<tenantId>-pf-tls` / `<tenantId>-pf-admin-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

All fields are optional. Unset fields use the Ping Identity defaults listed below.

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Ports

| Field | Env var | Default | Description |
|---|---|---|---|
| `enginePort` | `PF_ENGINE_PORT` | `9031` | HTTPS port for the PingFederate runtime engine. |
| `adminPort` | `PF_ADMIN_PORT` | `9999` | HTTPS port for the PingFederate admin console and API. |

#### Hostnames

| Field | Env var | Description |
|---|---|---|
| `enginePublicHostname` | `PF_ENGINE_PUBLIC_HOSTNAME` | Public hostname of the PF engine (used in redirects). |
| `adminPublicHostname` | `PF_ADMIN_PUBLIC_HOSTNAME` | Public hostname of the PF admin node. |
| `adminPublicBaseURL` | `PF_ADMIN_PUBLIC_BASEURL` | Full base URL of the PF admin console (e.g. `https://pf-admin.example.com:9999`). |

#### Console branding

| Field | Env var | Description |
|---|---|---|
| `consoleEnvironment` | `PF_CONSOLE_ENV` | Environment label shown in the admin console UI. |
| `consoleTitle` | `PF_CONSOLE_TITLE` | Title shown in the admin console. |

#### Operational mode

| Field | Env var | Default | Description |
|---|---|---|---|
| `operationalMode` | `OPERATIONAL_MODE` | `STANDALONE` | One of: `STANDALONE`, `CLUSTERED_CONSOLE`, `CLUSTERED_ENGINE`. |

#### Authentication

| Field | Env var | Default | Description |
|---|---|---|---|
| `consoleAuthentication` | `PF_CONSOLE_AUTHENTICATION` | `native` | Admin console auth mechanism. One of: `none`, `native`, `LDAP`, `cert`, `RADIUS`, `OIDC`. |
| `adminAPIAuthentication` | `PF_ADMIN_API_AUTHENTICATION` | `native` | Admin API auth mechanism. Same options as above. |
| `ldapType` | `PF_LDAP_TYPE` | `PingDirectory` | LDAP directory type when using LDAP auth. One of: `PingDirectory`, `ActiveDirectory`, `SunDirectoryServer`, `OracleUnifiedDirectory`, `Generic`. |
| `ldapUsername` | `PF_LDAP_USERNAME` | — | Username for LDAP lookups. |
| `ldapSecretRef` | — | — | Name of a Secret containing `PF_LDAP_PASSWORD`. Injected via `envFrom.secretRef`. |

#### PingOne integration

| Field | Env var | Description |
|---|---|---|
| `pingOneRegion` | `PF_PINGONE_REGION` | Region of the PingOne tenant. One of: `com`, `eu`, `asia`. |
| `pingOneEnvID` | `PF_PINGONE_ENV_ID` | PingOne environment ID to connect to. |

#### Provisioner

| Field | Env var | Default | Description |
|---|---|---|---|
| `provisionerMode` | `PF_PROVISIONER_MODE` | `OFF` | One of: `OFF`, `STANDALONE`, `FAILOVER`. |
| `provisionerNodeID` | `PF_PROVISIONER_NODE_ID` | `1` | Provisioner node ID. |

#### JVM and security

| Field | Env var | Default | Description |
|---|---|---|---|
| `javaRamPercentage` | `JAVA_RAM_PERCENTAGE` | `75.0` | Percentage of container memory allocated to the JVM. |
| `hsmMode` | `HSM_MODE` | `OFF` | Hardware Security Module mode. One of: `OFF`, `AWSCLOUDHSM`, `NCIPHER`, `LUNA`, `BCFIPS`. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `PING_IDENTITY_DEVOPS_USER`, `PING_IDENTITY_DEVOPS_KEY`, `PING_IDENTITY_PASSWORD`, and optionally `PF_LDAP_PASSWORD`. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingDirectory

Deployed by creating a `PingDirectory` CR. It renders as a **StatefulSet** workload.

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingDirectory
metadata:
  name: dev-pd
spec:
  environmentRef: env-dev
  replicas: 1
  storageSize: 8Gi
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      path: pingdirectory
    userBaseDN: "dc=myorg,dc=com"
```

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of StatefulSet pods. |
| `storageClass` | string | — | StorageClass for the `/opt/out` PersistentVolumeClaim. |
| `storageSize` | string | `8Gi` | PVC size for `/opt/out`. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### Ingress

PingDirectory speaks LDAP/LDAPS, not HTTP. Ingress is always disabled — if external LDAP access is required, use a `LoadBalancer` service or TCP passthrough via `valuesOverride`.

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Directory configuration

| Field | Env var | Default | Description |
|---|---|---|---|
| `userBaseDN` | `USER_BASE_DN` | `dc=example,dc=com` | Base DN for user data. |
| `replicationBaseDNs` | `REPLICATION_BASE_DNS` | — | Additional base DNs for replication, separated by `;`. |
| `replicationPort` | `REPLICATION_PORT` | `8989` | Port used for replication communication. |
| `adminUserName` | `ADMIN_USER_NAME` | `admin` | Replication admin user. |
| `makeLdifUsers` | `MAKELDIF_USERS` | `0` | Number of users to auto-generate on first start. |
| `retryTimeoutSeconds` | `RETRY_TIMEOUT_SECONDS` | `180` | Timeout for dsreplication operations. |

#### Ports

| Field | Env var | Default | Description |
|---|---|---|---|
| `ldapPort` | `LDAP_PORT` | `1389` | Container LDAP port. |
| `ldapsPort` | `LDAPS_PORT` | `1636` | Container LDAPS port. |
| `httpsPort` | `HTTPS_PORT` | `1443` | Container HTTPS port. |

#### Operational flags

| Field | Env var | Default | Description |
|---|---|---|---|
| `fipsModeOn` | `FIPS_MODE_ON` | `false` | Enable Bouncy Castle FIPS mode. |
| `rebuildOnRestart` | `PD_REBUILD_ON_RESTART` | `false` | Force replace-profile on every restart. |
| `parallelPodManagement` | `PARALLEL_POD_MANAGEMENT_POLICY` | `false` | Use `Parallel` StatefulSet podManagementPolicy. |
| `failOnDisabledBaseDN` | `FAIL_ON_DISABLED_BASE_DN` | `false` | Fail if `USER_BASE_DN` replication is not enabled. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `root-user-password`, `admin-user-password`, and `encryption-password`. |
| `keystoreSecretRef` | Name of a Secret containing the keystore (`keystore` key) and its pin (`keystore.pin` key) for TLS. Leave unset to auto-generate a self-signed cert. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingDataConsole

Deployed by creating a `PingDataConsole` CR. It renders as a Deployment workload.

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingDataConsole
metadata:
  name: dev-pd-console
spec:
  environmentRef: env-dev
  ingress:
    enabled: true
  config:
    brandingAppName: "Example Directory Console"
```

> **Deprecated:** the inline `spec.pingDataConsole` section on PingEnvironment still works
> (it deploys the console alongside a PingDirectory CR in the same environment), but new
> deployments should use the PingDataConsole CR. When both are present, the CR wins.

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this console belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of console pods. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.config

| Field | Env var | Default | Description |
|---|---|---|---|
| `httpPort` | `HTTP_PORT` | `8080` | HTTP listen port. |
| `httpsPort` | `HTTPS_PORT` | `8443` | HTTPS listen port. |
| `brandingAppName` | `BRANDING_APP_NAME` | `PingDirectory Admin Console` | Application name shown on the sign-on page and banner. |
| `systemReadOnly` | `SYSTEM_READ_ONLY` | `false` | Put the console in read-only mode. |
| `serverHost` | — | `<tenantId>-ping-pingdirectory-cluster` | PingData server hostname pre-filled on the sign-on page (`defaultLogin.server.host`). |
| `serverPort` | — | PingDirectory LDAPS port (`1636`) | LDAPS port pre-filled on the sign-on page (`defaultLogin.server.port`). |
| `envs` | — | — | Map of additional env vars, passed through verbatim. |
| `envConfigMapRef` | — | — | Name of a ConfigMap injected via `envFrom.configMapRef`. |

### spec.ingress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `pd-console.<domain>` | FQDN for PingDataConsole. |
| `tlsSecretRef` | string | `<tenantId>-pd-console-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

---

## PingAccess

Deployed by creating a `PingAccess` CR. It renders as two Deployment workloads: an admin console (`pingaccess-admin`) and one or more engines (`pingaccess-engine`).

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of engine pods. The admin always runs as a single pod. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.adminIngress / engineIngress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `pa-admin.<domain>` / `pa.<domain>` | FQDN for this component. |
| `tlsSecretRef` | string | `<tenantId>-pa-admin-tls` / `<tenantId>-pa-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Ports

| Field | Env var | Default | Description |
|---|---|---|---|
| `adminPort` | `PA_ADMIN_PORT` | `9000` | HTTPS port for the PingAccess admin console. |
| `enginePort` | `PA_ENGINE_PORT` | `3000` | HTTPS port for the PingAccess engine. |

#### Hostnames

| Field | Env var | Description |
|---|---|---|
| `adminPublicHostname` | `PA_ADMIN_PUBLIC_HOSTNAME` | Public hostname of the PA admin node. |
| `enginePublicHostname` | `PA_ENGINE_PUBLIC_HOSTNAME` | Public hostname of the PA engine node. |

#### Operational settings

| Field | Env var | Default | Description |
|---|---|---|---|
| `operationalMode` | `OPERATIONAL_MODE` | `STANDALONE` | One of: `STANDALONE`, `CLUSTERED_CONSOLE`, `CLUSTERED_ENGINE`. |
| `fipsModeOn` | `FIPS_MODE_ON` | `false` | Enable Bouncy Castle FIPS mode. |
| `javaRamPercentage` | `JAVA_RAM_PERCENTAGE` | `60.0` | Percentage of container memory allocated to the JVM. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `PA_ADMIN_PASSWORD`. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingAuthorize

Deployed by creating a `PingAuthorize` CR. It renders as a **StatefulSet** workload.

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of StatefulSet pods. |
| `storageClass` | string | — | StorageClass for the `/opt/out` PersistentVolumeClaim. |
| `storageSize` | string | `8Gi` | PVC size for `/opt/out`. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.ingress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `paz.<domain>` | FQDN for PingAuthorize. |
| `tlsSecretRef` | string | `<tenantId>-paz-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Directory settings

| Field | Env var | Default | Description |
|---|---|---|---|
| `userBaseDN` | `USER_BASE_DN` | `dc=example,dc=com` | Base DN for user data. |
| `adminUserName` | `ADMIN_USER_NAME` | `admin` | Admin user name. |
| `retryTimeoutSeconds` | `RETRY_TIMEOUT_SECONDS` | `180` | Timeout for startup operations. |

#### Ports

| Field | Env var | Default | Description |
|---|---|---|---|
| `ldapPort` | `LDAP_PORT` | `1389` | Container LDAP port. |
| `ldapsPort` | `LDAPS_PORT` | `1636` | Container LDAPS port. |
| `httpsPort` | `HTTPS_PORT` | `1443` | Container HTTPS port. |

#### JVM

| Field | Env var | Default | Description |
|---|---|---|---|
| `maxHeapSize` | `MAX_HEAP_SIZE` | `1g` | JVM max heap size. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `root-user-password` and `admin-user-password`. |
| `encryptionSecretRef` | Name of a Secret containing the encryption password for the policy database. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingAuthorizePAP

Deployed by creating a `PingAuthorizePAP` CR. It renders as a single-replica Deployment workload. No tier-based resource sizing is applied.

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.ingress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `paz-pap.<domain>` | FQDN for PingAuthorizePAP. |
| `tlsSecretRef` | string | `<tenantId>-paz-pap-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### PAP settings

| Field | Env var | Default | Description |
|---|---|---|---|
| `externalBaseURL` | `PING_EXTERNAL_BASE_URL` | derived from ingress hostname | External base URL for the PAP UI (e.g. `https://paz-pap.example.com`). Auto-derived from ingress hostname when not set. |
| `oidcConfigEndpoint` | `PING_OIDC_CONFIGURATION_ENDPOINT` | — | OIDC discovery endpoint for PAP authentication. |
| `clientId` | `PING_CLIENT_ID` | — | OIDC client ID. |
| `maxHeapSize` | `MAX_HEAP_SIZE` | `384m` | JVM max heap size. |
| `enableAPIHTTPCache` | `PING_ENABLE_API_HTTP_CACHE` | `true` | Enable the PAP API HTTP cache. |
| `policyDBSync` | `PING_POLICY_DB_SYNC` | `false` | Enable policy database synchronization. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `sharedSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `PING_SHARED_SECRET`. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingDataSync

Deployed by creating a `PingDataSync` CR. It renders as a **StatefulSet** workload.

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingDataSync
metadata:
  name: dev-pds
spec:
  environmentRef: env-dev
  replicas: 1
  config:
    serverProfile:
      url: https://github.com/pingidentity/pingidentity-server-profiles.git
      path: simple-sync/pingdatasync
```

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of StatefulSet pods. |
| `storageClass` | string | — | StorageClass for the `/opt/out` PersistentVolumeClaim. |
| `storageSize` | string | `8Gi` | PVC size for `/opt/out`. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.ingress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `pds.<domain>` | FQDN for PingDataSync. |
| `tlsSecretRef` | string | `<tenantId>-pds-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Sync configuration

| Field | Env var | Default | Description |
|---|---|---|---|
| `adminUserName` | `ADMIN_USER_NAME` | `admin` | Failover administrative user. |
| `retryTimeoutSeconds` | `RETRY_TIMEOUT_SECONDS` | `180` | Timeout for manage-topology operations. |
| `rebuildOnRestart` | `PD_REBUILD_ON_RESTART` | `false` | Force replace-profile on every restart. |
| `parallelPodManagement` | `PARALLEL_POD_MANAGEMENT_POLICY` | `false` | Use `Parallel` StatefulSet podManagementPolicy. |
| `skipWaitForDNS` | `SKIP_WAIT_FOR_DNS` | `false` | Skip the DNS readiness check on startup. |

#### TLS

| Field | Env var | Description |
|---|---|---|
| `certificateNickname` | `CERTIFICATE_NICKNAME` | Alias of the certificate to use within the keystore. |
| `keystoreFile` | `KEYSTORE_FILE` | Path to the keystore file. |
| `keystorePinFile` | `KEYSTORE_PIN_FILE` | Path to the keystore PIN file. |
| `keystoreType` | `KEYSTORE_TYPE` | Keystore format: `jks`, `pkcs12`, `pem`, or `bcfks`. |
| `truststoreFile` | `TRUSTSTORE_FILE` | Path to the truststore file. |
| `truststorePinFile` | `TRUSTSTORE_PIN_FILE` | Path to the truststore PIN file. |
| `truststoreType` | `TRUSTSTORE_TYPE` | Truststore format: `jks`, `pkcs12`, `pem`, or `bcfks`. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef` containing admin credentials (`ROOT_USER_PASSWORD_FILE` / `ADMIN_USER_PASSWORD_FILE`). |
| `keystoreSecretRef` | Name of a Secret containing the keystore file and PIN. |
| `truststoreSecretRef` | Name of a Secret containing the truststore file and PIN. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## PingDirectoryProxy

Deployed by creating a `PingDirectoryProxy` CR. It renders as a **StatefulSet** workload.

### Basic fields

| Field | Type | Default | Description |
|---|---|---|---|
| `environmentRef` | string | — | Name of the PingEnvironment this product belongs to. |
| `image` | string | — | Image repository override. |
| `version` | string | — | Image tag or full reference. |
| `replicas` | int | `1` | Number of StatefulSet pods. |
| `storageClass` | string | — | StorageClass for the `/opt/out` PersistentVolumeClaim. |
| `storageSize` | string | `8Gi` | PVC size for `/opt/out`. |
| `service` | object | — | Kubernetes Service customisation (`annotations`). |
| `container` | object | — | See [Container settings](#container-settings). |
| `valuesOverride` | object | — | Raw JSON deep-merged on top of all computed Helm values. |

### spec.ingress

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | inherited | Whether to create an Ingress resource. |
| `className` | string | inherited | `ingressClassName`. |
| `hostname` | string | `pdp.<domain>` | FQDN for PingDirectoryProxy. |
| `tlsSecretRef` | string | `<tenantId>-pdp-tls` | TLS Secret name. Auto-generated when not set. |
| `annotations` | map | merged from global | Additional Ingress annotations. |

### spec.config

#### Server profile

| Field | Env var | Description |
|---|---|---|
| `serverProfile` | `SERVER_PROFILE_*` | Base server profile. See [Server profiles](#server-profiles). |
| `serverProfileLayers` | `SERVER_PROFILE_<NAME>_*` | Layered profiles. See [Server profiles](#server-profiles). |

#### Proxy configuration

| Field | Env var | Default | Description |
|---|---|---|---|
| `adminUserName` | `ADMIN_USER_NAME` | `admin` | Replication administrative user. |
| `retryTimeoutSeconds` | `RETRY_TIMEOUT_SECONDS` | `180` | Timeout for manage-topology operations. |
| `pingDirectoryHostname` | `PINGDIRECTORY_HOSTNAME` | — | PingDirectory hostname used for automatic server discovery. |
| `pingDirectoryLDAPSPort` | `PINGDIRECTORY_LDAPS_PORT` | — | PingDirectory LDAPS port for automatic server discovery. |
| `joinPDTopology` | `JOIN_PD_TOPOLOGY` | `false` | Join the topology of PingDirectory. |

#### TLS

| Field | Env var | Description |
|---|---|---|
| `certificateNickname` | `CERTIFICATE_NICKNAME` | Alias of the certificate to use within the keystore. |
| `keystoreFile` | `KEYSTORE_FILE` | Path to the keystore file. |
| `keystorePinFile` | `KEYSTORE_PIN_FILE` | Path to the keystore PIN file. |
| `keystoreType` | `KEYSTORE_TYPE` | Keystore format: `jks`, `pkcs12`, `pem`, or `bcfks`. |
| `truststoreFile` | `TRUSTSTORE_FILE` | Path to the truststore file. |
| `truststorePinFile` | `TRUSTSTORE_PIN_FILE` | Path to the truststore PIN file. |
| `truststoreType` | `TRUSTSTORE_TYPE` | Truststore format: `jks`, `pkcs12`, `pem`, or `bcfks`. |

#### Extra env vars, Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef` containing admin credentials (`ROOT_USER_PASSWORD_FILE`). |
| `keystoreSecretRef` | Name of a Secret containing the keystore file and PIN. |
| `truststoreSecretRef` | Name of a Secret containing the truststore file and PIN. |
| `envs` | Map of additional env vars, passed through verbatim. Keys override built-in env vars. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## valuesOverride

Each product CR accepts a `spec.valuesOverride` field containing raw JSON that is deep-merged on top of all operator-computed Helm values. Override values always win. The values are scoped to the full `ping-devops` chart root — you can override any sub-chart key.

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingFederate
metadata:
  name: dev-pf
spec:
  environmentRef: env-dev
  valuesOverride:
    pingfederate-engine:
      container:
        resources:
          limits:
            cpu: "4"
            memory: "4Gi"
      envs:
        SOME_CUSTOM_VAR: "value"
```

When several product CRs define overrides, they are merged in a fixed order (PingFederate, PingDirectory, PingAccess, PingAuthorize, PingAuthorizePAP, PingDataSync, PingDirectoryProxy, PingDataConsole), so later products win on conflicting keys.

---

## Status fields

After reconciliation the PingEnvironment status reflects the outcome:

| Field | Description |
|---|---|
| `status.phase` | One of: `Pending`, `Deploying`, `Ready`, `Failed`. |
| `status.release` | Helm release name (`<tenantId>-ping`). |
| `status.conditions` | Standard Kubernetes conditions. `Ready=True` when deployment succeeded. |
| `status.observedGeneration` | Last spec generation processed by the reconciler. |

Each product CR mirrors the environment's outcome in its own status:

| Field | Description |
|---|---|
| `status.phase` | `Pending` while the referenced PingEnvironment does not exist, then `Ready` or `Failed` following the environment. |
| `status.release` | Helm release name that manages this product. |
| `status.conditions` / `status.observedGeneration` | As above. |

On failure the operator sets `phase=Failed` and requeues after 30 seconds. Fix the spec and re-apply to retry.

---

## Full example

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ping-dev
---
apiVersion: v1
kind: Secret
metadata:
  name: devops-secret
  namespace: ping-dev
type: Opaque
data:
  PING_IDENTITY_ACCEPT_EULA: eWVz
  PING_IDENTITY_DEVOPS_USER: <base64>
  PING_IDENTITY_DEVOPS_KEY: <base64>
---
apiVersion: pingone.io/v1alpha1
kind: PingEnvironment
metadata:
  name: myorg-dev
  namespace: ping-dev
spec:
  tenantId: myorg-dev
  tier: development
  domain: dev.myorg.example.com
  ingress:
    enabled: true
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
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  version: "13.0.2-edge"
  engineIngress:
    enabled: true
  adminIngress:
    enabled: true
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      branch: main
      path: pingfederate
    enginePublicHostname: pf.dev.myorg.example.com
    adminPublicHostname: pf-admin.dev.myorg.example.com
    adminPublicBaseURL: https://pf-admin.dev.myorg.example.com:9999
    consoleEnvironment: myorg-dev
    pingOneRegion: com
    pingOneEnvID: "abc123-def456"
---
apiVersion: pingone.io/v1alpha1
kind: PingDirectory
metadata:
  name: myorg-dev-pd
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  replicas: 1
  storageClass: standard
  storageSize: 10Gi
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      branch: main
      path: pingdirectory
    userBaseDN: "dc=myorg,dc=com"
    adminSecretRef: pd-admin-secret
---
apiVersion: pingone.io/v1alpha1
kind: PingDataConsole
metadata:
  name: myorg-dev-pd-console
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  ingress:
    enabled: true
---
apiVersion: pingone.io/v1alpha1
kind: PingAccess
metadata:
  name: myorg-dev-pa
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  engineIngress:
    enabled: true
  adminIngress:
    enabled: true
  container:
    waitFor:
      - application: pingFederate
        service: https
        timeoutSeconds: 300
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      path: getting-started/pingaccess
---
apiVersion: pingone.io/v1alpha1
kind: PingAuthorize
metadata:
  name: myorg-dev-paz
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  ingress:
    enabled: true
  container:
    waitFor:
      - application: pingDirectory
        service: ldaps
        timeoutSeconds: 300
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      path: paz-pap-integration/pingauthorize
      parent: baseline
    serverProfileLayers:
      - name: baseline
        url: https://github.com/myorg/ping-profiles.git
        path: baseline/pingauthorize
---
apiVersion: pingone.io/v1alpha1
kind: PingAuthorizePAP
metadata:
  name: myorg-dev-paz-pap
  namespace: ping-dev
spec:
  environmentRef: myorg-dev
  ingress:
    enabled: true
  config:
    serverProfile:
      url: https://github.com/myorg/ping-profiles.git
      path: paz-pap-integration/pingauthorizepap
    oidcConfigEndpoint: https://auth.myorg.example.com/.well-known/openid-configuration
    clientId: pingauthorize-pap
```
