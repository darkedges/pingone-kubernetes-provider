# Claude Code Prompt: PingOne Kubernetes Operator in Go

> Paste this into **Claude Code** at the root of your project. Run `kubebuilder init` and
> `kubebuilder create api` first to scaffold the project, then use this prompt to implement
> the full operator. All env var names and Helm values paths are sourced directly from the
> official Ping Identity DevOps documentation and the `ping-devops` Helm chart.

---

## Project Setup

| Setting          | Value                                                                                           |
| ---------------- | ----------------------------------------------------------------------------------------------- |
| Module name      | `github.com/<your-org>/pingone-operator`                                                        |
| Go version       | `1.22`                                                                                          |
| Key dependencies | `sigs.k8s.io/controller-runtime`, `helm.sh/helm/v3`, `k8s.io/client-go`                         |
| Project layout   | kubebuilder conventions                                                                         |
| Helm chart       | `ping-devops` from `https://helm.pingidentity.com` (single unified chart for all Ping products) |

---

## Custom Resource Definition

Create a CRD `PingEnvironment` in group `pingone.io/v1alpha1`:

```go
type PingEnvironmentSpec struct {
    // TenantID is the unique identifier from the PingOne API.
    // Used to namespace Helm release names (e.g. <tenantID>-pf, <tenantID>-pd).
    TenantID string `json:"tenantId"`

    // Tier controls resource sizing defaults. One of: development, staging, production.
    Tier string `json:"tier"`

    PingFederate  PingFederateSpec  `json:"pingFederate"`
    PingDirectory PingDirectorySpec `json:"pingDirectory"`

    // TargetNamespace is where both products will be deployed.
    // Defaults to the CR's own namespace.
    TargetNamespace string `json:"targetNamespace,omitempty"`
}

// PingFederateSpec configures the PingFederate deployment.
// Env var names map directly to the pingfederate container image env vars.
type PingFederateSpec struct {
    // Version sets global.image.tag in the Helm values.
    Version  string `json:"version"`
    Replicas int32  `json:"replicas"`

    // LicenseSecretRef is the name of a Secret containing the PingFederate
    // license file (key: pingfederate.lic). Mounted at LICENSE_DIR.
    LicenseSecretRef string `json:"licenseSecretRef"`

    Ingress IngressSpec          `json:"ingress,omitempty"`
    Config  PingFederateConfig   `json:"config,omitempty"`

    // ValuesOverride is merged last on top of all computed Helm values.
    ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingFederateConfig maps to the env vars consumed by the pingfederate container.
// See: https://developer.pingidentity.com/devops/docker-images/pingfederate/README.html
type PingFederateConfig struct {
    // SERVER_PROFILE_URL — Git HTTPS URL of the server profile repo.
    ServerProfileURL string `json:"serverProfileURL,omitempty"`

    // SERVER_PROFILE_BRANCH — Git branch to check out.
    ServerProfileBranch string `json:"serverProfileBranch,omitempty"`

    // SERVER_PROFILE_PATH — Subdirectory within the git repo.
    ServerProfilePath string `json:"serverProfilePath,omitempty"`

    // PF_ENGINE_PORT — HTTPS port for the PingFederate runtime engine. Default: 9031.
    EnginePort int32 `json:"enginePort,omitempty"`

    // PF_ADMIN_PORT — HTTPS port for the PingFederate admin console/API. Default: 9999.
    AdminPort int32 `json:"adminPort,omitempty"`

    // PF_ENGINE_PUBLIC_HOSTNAME — Public hostname of the PF engine node (used in redirects).
    EnginePublicHostname string `json:"enginePublicHostname,omitempty"`

    // PF_ADMIN_PUBLIC_HOSTNAME — Public hostname of the PF admin node.
    AdminPublicHostname string `json:"adminPublicHostname,omitempty"`

    // PF_ADMIN_PUBLIC_BASEURL — Full base URL of the PF admin console.
    // Example: https://pf-admin.example.com:9999
    AdminPublicBaseURL string `json:"adminPublicBaseURL,omitempty"`

    // PF_CONSOLE_ENV — Environment label shown in the admin console UI.
    ConsoleEnvironment string `json:"consoleEnvironment,omitempty"`

    // PF_CONSOLE_TITLE — Title shown in the admin console. Default: "Docker PingFederate".
    ConsoleTitle string `json:"consoleTitle,omitempty"`

    // OPERATIONAL_MODE — One of: STANDALONE, CLUSTERED_CONSOLE, CLUSTERED_ENGINE.
    // Default: STANDALONE.
    OperationalMode string `json:"operationalMode,omitempty"`

    // PF_CONSOLE_AUTHENTICATION — Admin console auth mechanism.
    // One of: none, native, LDAP, cert, RADIUS, OIDC. Default: native.
    ConsoleAuthentication string `json:"consoleAuthentication,omitempty"`

    // PF_ADMIN_API_AUTHENTICATION — Admin API auth mechanism.
    // One of: none, native, LDAP, cert, RADIUS, OIDC. Default: native.
    AdminAPIAuthentication string `json:"adminAPIAuthentication,omitempty"`

    // PF_LDAP_TYPE — LDAP directory type when using LDAP auth.
    // One of: PingDirectory, ActiveDirectory, SunDirectoryServer,
    // OracleUnifiedDirectory, Generic. Default: PingDirectory.
    LDAPType string `json:"ldapType,omitempty"`

    // PF_LDAP_USERNAME — Username for LDAP lookups when ConsoleAuthentication=LDAP.
    LDAPUsername string `json:"ldapUsername,omitempty"`

    // PF_PINGONE_REGION — Region of the PingOne tenant. One of: com, eu, asia.
    PingOneRegion string `json:"pingOneRegion,omitempty"`

    // PF_PINGONE_ENV_ID — PingOne environment ID to connect to.
    PingOneEnvID string `json:"pingOneEnvID,omitempty"`

    // PF_PROVISIONER_MODE — One of: OFF, STANDALONE, FAILOVER. Default: OFF.
    ProvisionerMode string `json:"provisionerMode,omitempty"`

    // PF_PROVISIONER_NODE_ID — Provisioner node ID. Default: 1.
    ProvisionerNodeID int32 `json:"provisionerNodeID,omitempty"`

    // JAVA_RAM_PERCENTAGE — Percentage of container memory for the JVM. Default: 75.0.
    // Do NOT set to 100 — the JVM will OOM.
    JavaRAMPercentage string `json:"javaRamPercentage,omitempty"`

    // HSM_MODE — Hardware Security Module mode.
    // One of: OFF, AWSCLOUDHSM, NCIPHER, LUNA, BCFIPS. Default: OFF.
    HSMMode string `json:"hsmMode,omitempty"`

    // AdminSecretRef — Secret containing PING_IDENTITY_DEVOPS_USER and
    // PING_IDENTITY_DEVOPS_KEY for license retrieval, and
    // PING_IDENTITY_PASSWORD for the admin user.
    AdminSecretRef string `json:"adminSecretRef,omitempty"`

    // LDAPSecretRef — Secret containing PF_LDAP_PASSWORD when ConsoleAuthentication=LDAP.
    LDAPSecretRef string `json:"ldapSecretRef,omitempty"`

    // EnvConfigMapRef — Name of a ConfigMap with additional env vars to inject.
    EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingDirectorySpec configures the PingDirectory StatefulSet.
type PingDirectorySpec struct {
    // Version sets global.image.tag in the Helm values.
    Version  string `json:"version"`
    Replicas int32  `json:"replicas"`

    // StorageClass for the /opt/out PersistentVolumeClaim. Maps to
    // workload.statefulSet.persistentvolume.volumes.out-dir.persistentVolumeClaim.storageClassName
    StorageClass string `json:"storageClass,omitempty"`

    // StorageSize for the /opt/out PVC. Default: 8Gi.
    StorageSize string `json:"storageSize,omitempty"`

    // LicenseSecretRef is the name of a Secret containing PingDirectory.lic.
    LicenseSecretRef string `json:"licenseSecretRef"`

    Ingress IngressSpec          `json:"ingress,omitempty"`
    Config  PingDirectoryConfig  `json:"config,omitempty"`

    ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingDirectoryConfig maps to the env vars consumed by the pingdirectory container.
// See: https://developer.pingidentity.com/devops/docker-images/pingdirectory/README.html
type PingDirectoryConfig struct {
    // SERVER_PROFILE_URL — Git HTTPS URL of the server profile repo.
    ServerProfileURL string `json:"serverProfileURL,omitempty"`

    // SERVER_PROFILE_BRANCH — Git branch to check out.
    ServerProfileBranch string `json:"serverProfileBranch,omitempty"`

    // SERVER_PROFILE_PATH — Subdirectory within the git repo.
    ServerProfilePath string `json:"serverProfilePath,omitempty"`

    // USER_BASE_DN — Base DN for user data. Default: dc=example,dc=com.
    UserBaseDN string `json:"userBaseDN,omitempty"`

    // REPLICATION_BASE_DNS — Additional base DNs for replication beyond USER_BASE_DN.
    // Separate multiple DNs with ";".
    ReplicationBaseDNs string `json:"replicationBaseDNs,omitempty"`

    // REPLICATION_PORT — Port used for replication communication. Default: 8989.
    ReplicationPort int32 `json:"replicationPort,omitempty"`

    // LDAP_PORT — Container LDAP port. Default: 1389.
    LDAPPort int32 `json:"ldapPort,omitempty"`

    // LDAPS_PORT — Container LDAPS port. Default: 1636.
    LDAPSPort int32 `json:"ldapsPort,omitempty"`

    // HTTPS_PORT — Container HTTPS port. Default: 1443.
    HTTPSPort int32 `json:"httpsPort,omitempty"`

    // ADMIN_USER_NAME — Replication admin user. Default: admin.
    AdminUserName string `json:"adminUserName,omitempty"`

    // MAKELDIF_USERS — Number of users to auto-generate on first start. Default: 0.
    MakeLdifUsers int32 `json:"makeLdifUsers,omitempty"`

    // RETRY_TIMEOUT_SECONDS — Timeout for dsreplication operations. Default: 180.
    RetryTimeoutSeconds int32 `json:"retryTimeoutSeconds,omitempty"`

    // FIPS_MODE_ON — Set to "true" to enable Bouncy Castle FIPS mode. Default: false.
    FIPSModeOn bool `json:"fipsModeOn,omitempty"`

    // PD_REBUILD_ON_RESTART — Force replace-profile on every restart. Default: false.
    RebuildOnRestart bool `json:"rebuildOnRestart,omitempty"`

    // PARALLEL_POD_MANAGEMENT_POLICY — Must be true when StatefulSet uses Parallel
    // podManagementPolicy. Requires RETRY_TIMEOUT_SECONDS to be large enough. Default: false.
    ParallelPodManagement bool `json:"parallelPodManagement,omitempty"`

    // FAIL_ON_DISABLED_BASE_DN — Fail the container if USER_BASE_DN replication is
    // not enabled. Default: false.
    FailOnDisabledBaseDN bool `json:"failOnDisabledBaseDN,omitempty"`

    // AdminSecretRef — Secret name containing:
    //   root-user-password (ROOT_USER_PASSWORD_FILE)
    //   admin-user-password (ADMIN_USER_PASSWORD_FILE)
    //   encryption-password (ENCRYPTION_PASSWORD_FILE)
    AdminSecretRef string `json:"adminSecretRef,omitempty"`

    // KeystoreSecretRef — Secret containing the keystore (KEYSTORE_FILE) and
    // its pin (KEYSTORE_PIN_FILE) for TLS. Leave unset to auto-generate self-signed.
    KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`

    // EnvConfigMapRef — Name of a ConfigMap with additional env vars to inject.
    EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// IngressSpec configures the Kubernetes Ingress for a product.
// Maps to the global.ingress / per-product ingress section in ping-devops values.
type IngressSpec struct {
    // Enabled maps to global.ingress.enabled.
    Enabled bool `json:"enabled"`

    // ClassName maps to global.ingress.spec.ingressClassName.
    // Example: nginx, traefik, alb.
    ClassName string `json:"className,omitempty"`

    // Hostname is the FQDN for this product.
    // Example: pf.abc123.yourdomain.com
    Hostname string `json:"hostname,omitempty"`

    // TLSSecretRef is the name of the TLS secret for this hostname.
    TLSSecretRef string `json:"tlsSecretRef,omitempty"`

    // Annotations are added to the Ingress resource.
    // Example: nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    Annotations map[string]string `json:"annotations,omitempty"`
}

// PingEnvironmentStatus reflects the observed state of the environment.
type PingEnvironmentStatus struct {
    // Phase is one of: Pending, Deploying, Ready, Failed.
    Phase string `json:"phase"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // PingFederateRelease is the Helm release name for PingFederate.
    PingFederateRelease string `json:"pingFederateRelease,omitempty"`

    // PingDirectoryRelease is the Helm release name for PingDirectory.
    PingDirectoryRelease string `json:"pingDirectoryRelease,omitempty"`

    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
```

---

## Reconciler Requirements

Implement `PingEnvironmentReconciler` with the following logic:

### 1. Fetch
Fetch the `PingEnvironment` CR. Return without error if not found (deleted).

### 2. Finalizer
- Add `pingone.io/cleanup` on first reconcile.
- On `DeletionTimestamp`, uninstall both Helm releases, then remove the finalizer.

### 3. Helm Client
Initialize a Helm `action.Configuration` scoped to `TargetNamespace` using the
in-cluster REST config. Use `helm.sh/helm/v3/pkg/action` and `helm.sh/helm/v3/pkg/chart/loader`.

### 4. Chart Source

> **Important:** Ping Identity publishes a single unified chart called `ping-devops`.
> Both PingFederate and PingDirectory are sub-charts within it. Do NOT use separate
> `pingfederate` or `pingdirectory` chart names.

```
Repo URL:   https://helm.pingidentity.com
Chart name: ping-devops
Cache dir:  /tmp/helm-cache
```

Each environment gets **one** Helm release of `ping-devops` named `<tenantID>-ping`,
with PingFederate and PingDirectory enabled/disabled via values.

### 5. Deploy Logic

| Step | Action                                                          |
| ---- | --------------------------------------------------------------- |
| a    | Check if release `<tenantID>-ping` exists                       |
| b    | If not → `helm install` with computed values                    |
| c    | If exists and `ObservedGeneration` changed → `helm upgrade`     |
| d    | Merge `ValuesOverride` JSON last, on top of all computed values |

### 6. Computed Helm Values

The operator must build a `map[string]any` that covers both sub-charts.
Below are the exact value paths as expected by the `ping-devops` chart:

#### 6a. Global / Shared Values

```yaml
global:
  envs:
    PING_IDENTITY_ACCEPT_EULA: "YES"                    # Required — container won't start without this
  ingress:
    enabled: <true if either product has ingress enabled>
    defaultDomain: <extract from hostname or set by operator config>
    spec:
      ingressClassName: <IngressSpec.ClassName>
  image:
    tag: <use per-product override; set in product section>
```

#### 6b. PingFederate Sub-Chart Values

PingFederate runs as a **Deployment** (default workload type). Map the CRD fields as follows:

```yaml
pingfederate:
  enabled: true
  workload:
    type: Deployment
    deployment:
      replicas: <spec.PingFederate.Replicas>
  image:
    tag: <spec.PingFederate.Version>
  container:
    resources:
      requests:
        cpu: "1"           # Scale by Tier: dev=500m, staging=1, prod=2
        memory: "1Gi"      # Scale by Tier: dev=512Mi, staging=1Gi, prod=2Gi
  envs:
    # Server Profile
    SERVER_PROFILE_URL:       <PingFederateConfig.ServerProfileURL>
    SERVER_PROFILE_BRANCH:    <PingFederateConfig.ServerProfileBranch>
    SERVER_PROFILE_PATH:      <PingFederateConfig.ServerProfilePath>
    SERVER_PROFILE_UPDATE:    "false"

    # Ports
    PF_ENGINE_PORT:           <PingFederateConfig.EnginePort | default 9031>
    PF_ADMIN_PORT:            <PingFederateConfig.AdminPort  | default 9999>

    # Hostnames
    PF_ENGINE_PUBLIC_HOSTNAME: <PingFederateConfig.EnginePublicHostname>
    PF_ADMIN_PUBLIC_HOSTNAME:  <PingFederateConfig.AdminPublicHostname>
    PF_ADMIN_PUBLIC_BASEURL:   <PingFederateConfig.AdminPublicBaseURL>

    # Console Branding
    PF_CONSOLE_ENV:   <PingFederateConfig.ConsoleEnvironment>
    PF_CONSOLE_TITLE: <PingFederateConfig.ConsoleTitle>

    # Operational Mode
    OPERATIONAL_MODE:          <PingFederateConfig.OperationalMode | default STANDALONE>
    CLUSTER_BIND_ADDRESS:      NON_LOOPBACK

    # Authentication
    PF_CONSOLE_AUTHENTICATION: <PingFederateConfig.ConsoleAuthentication | default native>
    PF_ADMIN_API_AUTHENTICATION: <PingFederateConfig.AdminAPIAuthentication | default native>
    PF_LDAP_TYPE:              <PingFederateConfig.LDAPType | default PingDirectory>
    PF_LDAP_USERNAME:          <PingFederateConfig.LDAPUsername>

    # PingOne Integration
    PF_PINGONE_REGION:  <PingFederateConfig.PingOneRegion>
    PF_PINGONE_ENV_ID:  <PingFederateConfig.PingOneEnvID>

    # Provisioner
    PF_PROVISIONER_MODE:         <PingFederateConfig.ProvisionerMode | default OFF>
    PF_PROVISIONER_NODE_ID:      <PingFederateConfig.ProvisionerNodeID | default 1>
    PF_PROVISIONER_GRACE_PERIOD: "600"

    # JVM Tuning
    JAVA_RAM_PERCENTAGE: <PingFederateConfig.JavaRAMPercentage | default 75.0>

    # HSM
    HSM_MODE: <PingFederateConfig.HSMMode | default OFF>

    # Logging
    TAIL_LOG_FILES: "${SERVER_ROOT_DIR}/log/server.log"

  services:
    https:
      containerPort: <EnginePort | 9031>
      servicePort:   <EnginePort | 9031>
      ingressPort:   443
      dataService:   true
    admin:
      containerPort: <AdminPort | 9999>
      servicePort:   <AdminPort | 9999>
      dataService:   true

  # License — mounted from the secret referenced by LicenseSecretRef
  # The secret must contain key: pingfederate.lic
  # Mount it as a volume at LICENSE_DIR = ${SERVER_ROOT_DIR}/server/default/conf
  secretVolumes:
    pingfederate-license:
      items:
        pingfederate.lic: pingfederate.lic
      mountPath: /opt/out/instance/server/default/conf
      secret: <spec.PingFederate.LicenseSecretRef>

  # envFrom — inject admin credentials and optional extra config
  envFrom:
    secretRef:
      - <spec.PingFederate.Config.AdminSecretRef>     # contains PING_IDENTITY_DEVOPS_USER,
                                                       # PING_IDENTITY_DEVOPS_KEY,
                                                       # PING_IDENTITY_PASSWORD,
                                                       # PF_LDAP_PASSWORD (if LDAP auth)
    configMapRef:
      - <spec.PingFederate.Config.EnvConfigMapRef>    # optional additional env vars

  ingress:
    enabled: <IngressSpec.Enabled>
    hosts:
      - host: <IngressSpec.Hostname>
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: <IngressSpec.TLSSecretRef>
        hosts:
          - <IngressSpec.Hostname>
    annotations: <IngressSpec.Annotations>
```

#### 6c. PingDirectory Sub-Chart Values

PingDirectory runs as a **StatefulSet**. Map the CRD fields as follows:

```yaml
pingdirectory:
  enabled: true
  workload:
    type: StatefulSet
    statefulSet:
      replicas: <spec.PingDirectory.Replicas>
      podManagementPolicy: <if Config.ParallelPodManagement then Parallel else OrderedReady>
      persistentvolume:
        enabled: true
        volumes:
          out-dir:
            mountPath: /opt/out
            persistentVolumeClaim:
              accessModes:
                - ReadWriteOnce
              storageClassName: <spec.PingDirectory.StorageClass>
              resources:
                requests:
                  storage: <spec.PingDirectory.StorageSize | default 8Gi>
  image:
    tag: <spec.PingDirectory.Version>
  container:
    resources:
      requests:
        cpu: "1"           # Scale by Tier: dev=500m, staging=1, prod=2
        memory: "2Gi"      # Scale by Tier: dev=1Gi, staging=2Gi, prod=4Gi
  envs:
    # Server Profile
    SERVER_PROFILE_URL:    <PingDirectoryConfig.ServerProfileURL>
    SERVER_PROFILE_BRANCH: <PingDirectoryConfig.ServerProfileBranch>
    SERVER_PROFILE_PATH:   <PingDirectoryConfig.ServerProfilePath>

    # Directory Config
    USER_BASE_DN:           <PingDirectoryConfig.UserBaseDN | default "dc=example,dc=com">
    REPLICATION_BASE_DNS:   <PingDirectoryConfig.ReplicationBaseDNs>
    REPLICATION_PORT:       <PingDirectoryConfig.ReplicationPort | default 8989>
    ADMIN_USER_NAME:        <PingDirectoryConfig.AdminUserName | default admin>
    MAKELDIF_USERS:         <PingDirectoryConfig.MakeLdifUsers | default 0>
    RETRY_TIMEOUT_SECONDS:  <PingDirectoryConfig.RetryTimeoutSeconds | default 180>

    # Ports
    LDAP_PORT:  <PingDirectoryConfig.LDAPPort  | default 1389>
    LDAPS_PORT: <PingDirectoryConfig.LDAPSPort | default 1636>
    HTTPS_PORT: <PingDirectoryConfig.HTTPSPort | default 1443>

    # Operational flags
    FIPS_MODE_ON:                   <PingDirectoryConfig.FIPSModeOn | default false>
    PD_REBUILD_ON_RESTART:          <PingDirectoryConfig.RebuildOnRestart | default false>
    FAIL_ON_DISABLED_BASE_DN:       <PingDirectoryConfig.FailOnDisabledBaseDN | default false>
    PARALLEL_POD_MANAGEMENT_POLICY: <PingDirectoryConfig.ParallelPodManagement | default false>
    UNBOUNDID_SKIP_START_PRECHECK_NODETACH: "true"

    # Performance tuning
    JAVA_RAM_PERCENTAGE: "75.0"

    # Logging
    TAIL_LOG_FILES: >-
      ${SERVER_ROOT_DIR}/logs/access
      ${SERVER_ROOT_DIR}/logs/errors
      ${SERVER_ROOT_DIR}/logs/failed-ops
      ${SERVER_ROOT_DIR}/logs/config-audit.log

  services:
    ldap:
      containerPort: <LDAPPort  | 1389>
      servicePort:   <LDAPPort  | 1389>
      clusterService: true        # headless — LDAP clients need direct pod addressing
    ldaps:
      containerPort: <LDAPSPort | 1636>
      servicePort:   <LDAPSPort | 1636>
      clusterService: true
    https:
      containerPort: <HTTPSPort | 1443>
      servicePort:   <HTTPSPort | 1443>
      dataService:   true

  # License — must contain key: PingDirectory.lic
  secretVolumes:
    pingdirectory-license:
      items:
        PingDirectory.lic: PingDirectory.lic
      mountPath: /opt/staging/pd.profile/server-root/pre-setup
      secret: <spec.PingDirectory.LicenseSecretRef>

  # envFrom — inject admin credentials and optional extra config
  envFrom:
    secretRef:
      - <spec.PingDirectory.Config.AdminSecretRef>    # must contain keys:
                                                       # root-user-password
                                                       # admin-user-password
                                                       # encryption-password
    configMapRef:
      - <spec.PingDirectory.Config.EnvConfigMapRef>   # optional additional env vars

  # Keystore — optional; if unset, container auto-generates a self-signed cert
  # Secret must contain: keystore (file), keystore.pin (pin file)
  # Set KEYSTORE_FILE, KEYSTORE_PIN_FILE, KEYSTORE_TYPE via envs if provided

  ingress:
    enabled: false    # PingDirectory speaks LDAP/LDAPS — not HTTP.
                      # If external LDAPS access is needed, use a LoadBalancer service
                      # or TCP passthrough via ingress controller instead.
```

---

## Additional Files to Generate

### `internal/helm/client.go`

Implement a `NewHelmClient(namespace string, restConfig *rest.Config) (*action.Configuration, error)` constructor using the in-cluster REST config. The client must be namespace-scoped.

### `internal/helm/release.go`

Implement these helpers:

```go
// ReleaseExists returns true if a Helm release with the given name exists
// in a non-failed state.
func ReleaseExists(cfg *action.Configuration, releaseName string) (bool, error)

// BuildPingValues constructs the full Helm values map for a ping-devops
// release from a PingEnvironment spec. Call MergeValues last.
func BuildPingValues(spec pingonev1alpha1.PingEnvironmentSpec) (map[string]any, error)

// MergeValues deep-merges override (a runtime.RawExtension containing JSON)
// on top of base. The override wins on conflicts.
func MergeValues(base map[string]any, override runtime.RawExtension) (map[string]any, error)

// TierResources returns CPU/memory resource requests for a given tier string.
// Tiers: development, staging, production.
func TierResources(tier string) (cpu, memory string)
```

### `internal/helm/defaults.go`

Implement `applyDefaults(spec *pingonev1alpha1.PingEnvironmentSpec)` which fills in zero-value fields with the official Ping Identity defaults listed in the CRD comments above, before values are built.

### `main.go`

Standard `controller-runtime` manager with:
- Leader election enabled
- Metrics on `:8080`
- Health probe on `:8081`

### `config/samples/pingone_v1alpha1_pingenvironment.yaml`

A realistic sample CR for a **development** environment:

```yaml
apiVersion: pingone.io/v1alpha1
kind: PingEnvironment
metadata:
  name: env-dev-abc123
  namespace: ping-environments
spec:
  tenantId: abc123
  tier: development
  targetNamespace: ping-abc123

  pingFederate:
    version: "12.1"
    replicas: 1
    licenseSecretRef: pf-license-abc123
    ingress:
      enabled: true
      className: nginx
      hostname: pf.abc123.dev.example.com
      tlsSecretRef: pf-tls-abc123
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
        nginx.ingress.kubernetes.io/ssl-redirect: "true"
    config:
      serverProfileURL: https://github.com/your-org/ping-profiles.git
      serverProfilePath: pingfederate
      enginePort: 9031
      adminPort: 9999
      enginePublicHostname: pf.abc123.dev.example.com
      adminPublicHostname: pf-admin.abc123.dev.example.com
      adminPublicBaseURL: https://pf-admin.abc123.dev.example.com:9999
      consoleEnvironment: abc123-dev
      consoleTitle: "Dev - abc123"
      operationalMode: STANDALONE
      consoleAuthentication: native
      adminAPIAuthentication: native
      ldapType: PingDirectory
      pingOneRegion: com
      provisionerMode: OFF
      javaRamPercentage: "75.0"
      adminSecretRef: pf-admin-secret-abc123

  pingDirectory:
    version: "9.3"
    replicas: 2
    storageClass: standard
    storageSize: 8Gi
    licenseSecretRef: pd-license-abc123
    config:
      serverProfileURL: https://github.com/your-org/ping-profiles.git
      serverProfilePath: pingdirectory
      userBaseDN: "dc=abc123,dc=com"
      replicationPort: 8989
      ldapPort: 1389
      ldapsPort: 1636
      httpsPort: 1443
      adminUserName: admin
      retryTimeoutSeconds: 180
      parallelPodManagement: false
      failOnDisabledBaseDN: false
      adminSecretRef: pd-admin-secret-abc123
```

---

## Reconciler Error Handling

| Condition                      | Behavior                                                                |
| ------------------------------ | ----------------------------------------------------------------------- |
| Helm install/upgrade error     | Set `Phase=Failed`, condition message = error string, requeue after 30s |
| Transient Kubernetes API error | Return `err` — controller requeues with exponential backoff             |
| CR not found                   | Return `nil` — already deleted                                          |
| Polling during deploy          | `ctrl.Result{RequeueAfter: 30 * time.Second}`                           |

---

## RBAC Markers

```go
//+kubebuilder:rbac:groups=pingone.io,resources=pingenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;configmaps;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
```

---

## Code Style Requirements

- All exported types and functions must have godoc comments
- Use structured logging via `logr` (`log.FromContext(ctx)`)
- No panics — all errors must be returned or handled
- Keep the reconciler under **250 lines** by delegating to `internal/helm` helpers
- Use `client.MergeFrom` for status patches to avoid resource version conflicts
- Use `meta.SetStatusCondition` for all condition updates

---

## Suggested Follow-up Prompts

After Claude Code scaffolds the project, use these targeted follow-ups:

```
Implement internal/helm/client.go — NewHelmClient scoped to a namespace using
the in-cluster REST config and helm action.Configuration
```

```
Implement internal/helm/release.go — BuildPingValues must produce the full
ping-devops values map from a PingEnvironment spec, including all envs,
services, secretVolumes, envFrom, ingress, and workload sections
```

```
Implement the finalizer logic in the reconciler — on DeletionTimestamp, uninstall
the ping-devops Helm release <tenantID>-ping using helm action.Uninstall, update
status to Phase=Pending, then remove the pingone.io/cleanup finalizer
```

```
Implement TierResources in internal/helm/defaults.go — development returns
cpu=500m/memory=512Mi for PF and cpu=500m/memory=1Gi for PD,
staging doubles those, production doubles staging
```

---

## Key Architecture Notes for Claude Code

**Single Helm release per environment:** Both PingFederate and PingDirectory are sub-charts
of `ping-devops`. One release (`<tenantID>-ping`) per environment, not two separate releases.

**PingDirectory is not HTTP:** Do not enable the standard Kubernetes Ingress for PingDirectory.
If external LDAP access is required, configure a `LoadBalancer` service or TCP passthrough
via an ingress controller annotation. The ingress in the CRD is stubbed out but should
emit a warning if `enabled: true` is set for PingDirectory.

**Secret structure matters:** The container images expect secrets mounted at specific paths
(`/run/secrets/` by default). When using `secretRef` in `envFrom`, the secret keys must
match the env var names exactly (e.g. `PING_IDENTITY_DEVOPS_USER`, `root-user-password`).

**PING_IDENTITY_ACCEPT_EULA must be YES:** Neither container will start without this.
The operator should always inject it and never allow the user to override it to anything
other than `YES`.