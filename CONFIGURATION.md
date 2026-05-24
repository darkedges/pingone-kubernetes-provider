# PingOne Operator — Configuration Guide

The operator manages Ping Identity products in Kubernetes via a single `PingEnvironment` custom resource. Each CR maps to one Helm release of the `ping-devops` chart (version `0.12.2` from `https://helm.pingidentity.com`).

---

## How it works

1. You apply a `PingEnvironment` manifest to the cluster.
2. The operator reconciles it by building a `ping-devops` Helm values map from the spec.
3. A single Helm release named `<tenantId>-ping` is installed or upgraded in the target namespace.
4. PingFederate is always deployed. PingDirectory is optional — omit `spec.pingDirectory` to skip it.
5. All Ping Identity container env vars are driven from the CRD fields; you never write raw Helm values unless you need an escape hatch via `valuesOverride`.

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

## Full resource structure

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingEnvironment
metadata:
  name: <name>
  namespace: <namespace>
spec:
  tenantId: <string>           # required
  tier: <string>               # required: development | staging | production
  targetNamespace: <string>    # optional, defaults to metadata.namespace

  pingFederate:                # required
    version: <string>
    replicas: <int>
    ingress: { ... }           # optional
    config: { ... }            # optional
    valuesOverride: { ... }    # optional

  pingDirectory:               # optional — omit entirely to skip PingDirectory
    version: <string>
    replicas: <int>
    storageClass: <string>     # optional
    storageSize: <string>      # optional, default: 8Gi
    ingress: { ... }           # optional
    config: { ... }            # optional
    valuesOverride: { ... }    # optional
```

---

## Global fields

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.tenantId` | string | Yes | Unique identifier for this environment. Used as the Helm release name prefix (`<tenantId>-ping`). |
| `spec.tier` | string | Yes | One of `development`, `staging`, `production`. Controls CPU/memory resource requests for all products. |
| `spec.targetNamespace` | string | No | Namespace to deploy the Helm release into. Defaults to the CR's own namespace. |

### Resource sizing by tier

| Tier | PingFederate CPU | PingFederate Memory | PingDirectory CPU | PingDirectory Memory |
|---|---|---|---|---|
| `development` | 500m | 512Mi | 500m | 1Gi |
| `staging` | 1 | 1Gi | 1 | 2Gi |
| `production` | 2 | 2Gi | 2 | 4Gi |

---

## spec.pingFederate

PingFederate is deployed as a **Deployment** workload via the `pingfederate` sub-chart.

### Basic fields

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | string | Yes | Container image tag (e.g. `13.0.2-edge`). Sets `pingfederate.image.tag`. |
| `replicas` | int | Yes | Number of pods. Sets `pingfederate.workload.deployment.replicas`. |
| `valuesOverride` | object | No | Raw JSON merged last on top of all computed Helm values. Use as an escape hatch for chart options not exposed by the CRD. |

### spec.pingFederate.ingress

Controls the Kubernetes Ingress for PingFederate.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Whether to create an Ingress resource. Also sets `global.ingress.enabled`. |
| `className` | string | — | `ingressClassName` (e.g. `nginx`, `traefik`, `alb`). |
| `hostname` | string | — | FQDN for PingFederate (e.g. `pf.example.com`). |
| `tlsSecretRef` | string | — | Name of the TLS Secret for this hostname. |
| `annotations` | map | — | Annotations added to the Ingress resource. |

Example:
```yaml
ingress:
  enabled: true
  className: nginx
  hostname: pf.myorg.example.com
  tlsSecretRef: pf-tls-secret
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
```

### spec.pingFederate.config

Maps CRD fields to PingFederate container environment variables. All fields are optional; unset fields use the Ping Identity defaults listed below.

#### Server profile

| Field | Env var | Default | Description |
|---|---|---|---|
| `serverProfileURL` | `SERVER_PROFILE_URL` | — | Git HTTPS URL of the server profile repo. |
| `serverProfileBranch` | `SERVER_PROFILE_BRANCH` | — | Git branch to check out. |
| `serverProfilePath` | `SERVER_PROFILE_PATH` | — | Subdirectory within the repo. |

#### Ports

| Field | Env var | Default | Description |
|---|---|---|---|
| `enginePort` | `PF_ENGINE_PORT` | `9031` | HTTPS port for the PingFederate runtime engine. |
| `adminPort` | `PF_ADMIN_PORT` | `9999` | HTTPS port for the PingFederate admin console and API. |

#### Hostnames

| Field | Env var | Default | Description |
|---|---|---|---|
| `enginePublicHostname` | `PF_ENGINE_PUBLIC_HOSTNAME` | — | Public hostname of the PF engine node (used in redirects). |
| `adminPublicHostname` | `PF_ADMIN_PUBLIC_HOSTNAME` | — | Public hostname of the PF admin node. |
| `adminPublicBaseURL` | `PF_ADMIN_PUBLIC_BASEURL` | — | Full base URL of the PF admin console (e.g. `https://pf-admin.example.com:9999`). |

#### Console branding

| Field | Env var | Default | Description |
|---|---|---|---|
| `consoleEnvironment` | `PF_CONSOLE_ENV` | — | Environment label shown in the admin console UI. |
| `consoleTitle` | `PF_CONSOLE_TITLE` | — | Title shown in the admin console. |

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

| Field | Env var | Default | Description |
|---|---|---|---|
| `pingOneRegion` | `PF_PINGONE_REGION` | — | Region of the PingOne tenant. One of: `com`, `eu`, `asia`. |
| `pingOneEnvID` | `PF_PINGONE_ENV_ID` | — | PingOne environment ID to connect to. |

#### Provisioner

| Field | Env var | Default | Description |
|---|---|---|---|
| `provisionerMode` | `PF_PROVISIONER_MODE` | `OFF` | One of: `OFF`, `STANDALONE`, `FAILOVER`. |
| `provisionerNodeID` | `PF_PROVISIONER_NODE_ID` | `1` | Provisioner node ID. |

#### JVM and security

| Field | Env var | Default | Description |
|---|---|---|---|
| `javaRamPercentage` | `JAVA_RAM_PERCENTAGE` | `75.0` | Percentage of container memory allocated to the JVM. Do not set to `100`. |
| `hsmMode` | `HSM_MODE` | `OFF` | Hardware Security Module mode. One of: `OFF`, `AWSCLOUDHSM`, `NCIPHER`, `LUNA`, `BCFIPS`. |

#### Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `PING_IDENTITY_DEVOPS_USER`, `PING_IDENTITY_DEVOPS_KEY`, `PING_IDENTITY_PASSWORD`, and optionally `PF_LDAP_PASSWORD`. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## spec.pingDirectory

PingDirectory is **optional**. Omit this section entirely to skip PingDirectory deployment. When present, PingDirectory is deployed as a **StatefulSet** workload.

### Basic fields

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | string | Yes | Container image tag (e.g. `11.0.0.2-edge`). Sets `pingdirectory.image.tag`. |
| `replicas` | int | Yes | Number of pods. Sets `pingdirectory.workload.statefulSet.replicas`. |
| `storageClass` | string | No | StorageClass for the `/opt/out` PersistentVolumeClaim. |
| `storageSize` | string | No | PVC size for `/opt/out`. Default: `8Gi`. |
| `valuesOverride` | object | No | Raw JSON merged last on top of all computed Helm values. |

### spec.pingDirectory.ingress

PingDirectory speaks LDAP/LDAPS, not HTTP. The standard Kubernetes Ingress is always disabled (`enabled: false` is forced). If external LDAP access is required, use a `LoadBalancer` service or TCP passthrough via ingress controller annotations in `valuesOverride`.

The `ingress` block is available in the CRD for completeness but its `enabled` field is ignored by the operator.

### spec.pingDirectory.config

Maps CRD fields to PingDirectory container environment variables. All fields are optional.

#### Server profile

| Field | Env var | Default | Description |
|---|---|---|---|
| `serverProfileURL` | `SERVER_PROFILE_URL` | — | Git HTTPS URL of the server profile repo. |
| `serverProfileBranch` | `SERVER_PROFILE_BRANCH` | — | Git branch to check out. |
| `serverProfilePath` | `SERVER_PROFILE_PATH` | — | Subdirectory within the repo. |

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
| `parallelPodManagement` | `PARALLEL_POD_MANAGEMENT_POLICY` | `false` | Use `Parallel` StatefulSet podManagementPolicy. Also sets `statefulSet.podManagementPolicy`. |
| `failOnDisabledBaseDN` | `FAIL_ON_DISABLED_BASE_DN` | `false` | Fail if `USER_BASE_DN` replication is not enabled. |

#### Secret and ConfigMap references

| Field | Description |
|---|---|
| `adminSecretRef` | Name of a Secret injected via `envFrom.secretRef`. Should contain `root-user-password`, `admin-user-password`, and `encryption-password`. |
| `keystoreSecretRef` | Name of a Secret containing the keystore (`keystore` key) and its pin (`keystore.pin` key) for TLS. Leave unset to auto-generate a self-signed cert. |
| `envConfigMapRef` | Name of a ConfigMap injected via `envFrom.configMapRef` for additional env vars. |

---

## valuesOverride

Both `spec.pingFederate.valuesOverride` and `spec.pingDirectory.valuesOverride` accept raw JSON that is deep-merged on top of all operator-computed Helm values. Override values always win.

The values are scoped to the full `ping-devops` chart — you can override any key the chart accepts.

```yaml
pingFederate:
  valuesOverride:
    pingfederate:
      container:
        resources:
          limits:
            cpu: "4"
            memory: "4Gi"
      envs:
        SOME_CUSTOM_VAR: "value"
```

---

## Full example — development environment

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
  targetNamespace: ping-dev

  pingFederate:
    version: "13.0.2-edge"
    replicas: 1
    ingress:
      enabled: true
      className: nginx
      hostname: pf.dev.myorg.example.com
      tlsSecretRef: pf-tls
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    config:
      serverProfileURL: https://github.com/myorg/ping-profiles.git
      serverProfileBranch: main
      serverProfilePath: pingfederate
      enginePublicHostname: pf.dev.myorg.example.com
      adminPublicHostname: pf-admin.dev.myorg.example.com
      adminPublicBaseURL: https://pf-admin.dev.myorg.example.com:9999
      consoleEnvironment: myorg-dev
      operationalMode: STANDALONE
      pingOneRegion: com
      pingOneEnvID: "abc123-def456"

  pingDirectory:
    version: "11.0.0.2-edge"
    replicas: 1
    storageClass: standard
    storageSize: 10Gi
    config:
      serverProfileURL: https://github.com/myorg/ping-profiles.git
      serverProfileBranch: main
      serverProfilePath: pingdirectory
      userBaseDN: "dc=myorg,dc=com"
      adminSecretRef: pd-admin-secret
```

---

## Full example — production environment with PingFederate only

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingEnvironment
metadata:
  name: myorg-prod
  namespace: ping-prod
spec:
  tenantId: myorg-prod
  tier: production

  pingFederate:
    version: "13.0.2-edge"
    replicas: 3
    ingress:
      enabled: true
      className: nginx
      hostname: pf.myorg.example.com
      tlsSecretRef: pf-tls-prod
    config:
      enginePublicHostname: pf.myorg.example.com
      adminPublicHostname: pf-admin.myorg.example.com
      adminPublicBaseURL: https://pf-admin.myorg.example.com:9999
      consoleEnvironment: production
      operationalMode: CLUSTERED_ENGINE
      pingOneRegion: com
      pingOneEnvID: "prod-env-id"
      provisionerMode: STANDALONE
      adminSecretRef: pf-admin-secret
    valuesOverride:
      pingfederate:
        container:
          resources:
            limits:
              cpu: "4"
              memory: "4Gi"
  # pingDirectory omitted — not deployed
```

---

## Status fields

After reconciliation the CR's status reflects the outcome:

| Field | Description |
|---|---|
| `status.phase` | One of: `Pending`, `Deploying`, `Ready`, `Failed`. |
| `status.pingFederateRelease` | Helm release name (`<tenantId>-ping`). |
| `status.pingDirectoryRelease` | Helm release name (`<tenantId>-ping`) if PingDirectory is enabled, otherwise empty. |
| `status.conditions` | Standard Kubernetes conditions. `Ready=True` when deployment succeeded. |
| `status.observedGeneration` | Last spec generation processed by the reconciler. |

On failure the operator sets `phase=Failed` and requeues after 30 seconds. Fix the spec and re-apply to retry.
