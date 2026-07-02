package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GlobalServicesSpec holds Kubernetes Service settings shared across all components.
// Per-component ServiceSpec annotations take precedence on conflict.
type GlobalServicesSpec struct {
	// Annotations are merged into every component's Service resources.
	// Per-component service.annotations take precedence on conflict.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ServiceSpec holds per-product Kubernetes Service customisation.
type ServiceSpec struct {
	// Annotations are merged on top of the global service annotations for this component.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// GlobalIngressSpec holds ingress settings shared across all components.
// Per-component IngressSpec fields override these when set.
type GlobalIngressSpec struct {
	// Enabled turns ingress on for all components by default.
	// Each component can still override with its own enabled field.
	Enabled bool `json:"enabled,omitempty"`
	// ClassName is the ingressClassName applied to all component ingresses (e.g. nginx, traefik, alb).
	ClassName string `json:"className,omitempty"`
	// TLSSecretRef is the default TLS secret name. Per-component tlsSecretRef overrides this.
	TLSSecretRef string `json:"tlsSecretRef,omitempty"`
	// Annotations are merged into every component's Ingress resource. Per-component annotations take precedence on conflict.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// IngressSpec configures the Kubernetes Ingress for a product.
// Fields left empty inherit from spec.ingress (GlobalIngressSpec).
type IngressSpec struct {
	// Enabled controls whether an Ingress resource is created for this component.
	// When nil the value is inherited from spec.ingress.enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// ClassName overrides the global ingressClassName for this component.
	ClassName string `json:"className,omitempty"`
	// Hostname is the FQDN for this component. Defaults to <prefix>.<spec.domain> when spec.domain is set.
	Hostname string `json:"hostname,omitempty"`
	// TLSSecretRef overrides the global TLS secret name for this component.
	TLSSecretRef string `json:"tlsSecretRef,omitempty"`
	// Annotations are merged on top of the global annotations for this component.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// WaitForSpec defines a single service dependency that must be reachable before the container starts.
type WaitForSpec struct {
	// Application is the logical name of the Ping product to wait for.
	// Supported values: pingDirectory, pingFederate, pingFederateAdmin, pingFederateEngine,
	// pingAccess, pingAccessAdmin, pingAccessEngine, pingAuthorize, pingAuthorizePAP, pingDataConsole.
	// The operator resolves this to the correct Helm sub-chart service name automatically.
	Application string `json:"application"`
	// Service is the port/service type to probe (e.g. ldaps, https, ldap).
	Service string `json:"service"`
	// TimeoutSeconds is how long to wait before giving up. Default: 300.
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// ResourceRequirementsSpec defines CPU and memory requests and limits.
type ResourceRequirementsSpec struct {
	// Limits are the maximum resources the container may use.
	Limits map[string]string `json:"limits,omitempty"`
	// Requests are the minimum resources the container requires.
	Requests map[string]string `json:"requests,omitempty"`
}

// ContainerSpec holds container-level settings that apply on top of the chart defaults.
type ContainerSpec struct {
	// WaitFor is a list of services this container waits for before starting.
	WaitFor []WaitForSpec `json:"waitFor,omitempty"`
	// IncludeVolumes is a list of volume names defined in PingEnvironment.spec.volumes
	// to mount into this product's pod.
	IncludeVolumes []string `json:"includeVolumes,omitempty"`
	// Resources overrides the CPU/memory requests and limits for this product's containers.
	// Takes precedence over the tier-based defaults and PingEnvironment.spec.resources.
	Resources *ResourceRequirementsSpec `json:"resources,omitempty"`
	// SecurityContext overrides the pod-level securityContext for this product's workload.
	// +kubebuilder:pruning:PreserveUnknownFields
	SecurityContext *runtime.RawExtension `json:"securityContext,omitempty"`
	// ContainerSecurityContext overrides the container-level securityContext for this product.
	// +kubebuilder:pruning:PreserveUnknownFields
	ContainerSecurityContext *runtime.RawExtension `json:"containerSecurityContext,omitempty"`
	// Volumes is a list of additional pod-level volumes for this product's workload.
	// Each element is a standard Kubernetes volume spec (name, configMap, secret, emptyDir, etc.).
	// Maps directly to the ping-devops sub-chart volumes array.
	// +kubebuilder:pruning:PreserveUnknownFields
	Volumes []runtime.RawExtension `json:"volumes,omitempty"`
	// VolumeMounts is a list of additional container volumeMounts for this product's workload.
	// Each element is a standard Kubernetes volumeMount spec (name, mountPath, subPath, etc.).
	// Maps directly to the ping-devops sub-chart volumeMounts array.
	// +kubebuilder:pruning:PreserveUnknownFields
	VolumeMounts []runtime.RawExtension `json:"volumeMounts,omitempty"`
}

// ServerProfileSpec defines the base server profile for a container.
// See: https://developer.pingidentity.com/devops/how-to/profilesLayered.html
//
// Maps to SERVER_PROFILE_URL / _BRANCH / _PATH / _PARENT.
// Use serverProfileLayers for additional named layers chained via parent.
type ServerProfileSpec struct {
	// URL is the Git HTTPS URL of the server profile repo.
	URL string `json:"url,omitempty"`
	// Branch is the Git branch to check out. Omit to use the repo default branch.
	Branch string `json:"branch,omitempty"`
	// Path is the subdirectory within the git repo.
	Path string `json:"path,omitempty"`
	// Parent is the name of a layer in serverProfileLayers that is applied beneath this profile.
	// Sets SERVER_PROFILE_PARENT.
	Parent string `json:"parent,omitempty"`
}

// ServerProfileLayerSpec defines a named server profile layer for layered profiles.
// Each entry maps to SERVER_PROFILE_<UPPER(NAME)>_URL / _BRANCH / _PATH / _PARENT.
type ServerProfileLayerSpec struct {
	// Name is the layer identifier (e.g. paz, base, extensions).
	// Env vars use the uppercased name: paz → SERVER_PROFILE_PAZ_URL etc.
	Name string `json:"name"`
	// URL is the Git HTTPS URL of this layer's repo.
	URL string `json:"url,omitempty"`
	// Branch is the Git branch to check out.
	Branch string `json:"branch,omitempty"`
	// Path is the subdirectory within the git repo.
	Path string `json:"path,omitempty"`
	// Parent is the name of a deeper layer in serverProfileLayers (sets SERVER_PROFILE_<NAME>_PARENT).
	Parent string `json:"parent,omitempty"`
}

// PingAccessConfig maps to the env vars consumed by the pingaccess container.
// See: https://developer.pingidentity.com/devops/docker-images/pingaccess/README.html
type PingAccessConfig struct {
	// ServerProfile is the primary server profile for PingAccess.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers (layered profiles).
	// Keys are the layer names (e.g. "BASE"); env vars use the uppercased key.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// AdminPort is the HTTPS port for the PingAccess admin console (PA_ADMIN_PORT). Default: 9000.
	AdminPort int32 `json:"adminPort,omitempty"`
	// EnginePort is the HTTPS port for the PingAccess engine (PA_ENGINE_PORT). Default: 3000.
	EnginePort int32 `json:"enginePort,omitempty"`
	// AdminPublicHostname is the public hostname of the PA admin node (PA_ADMIN_PUBLIC_HOSTNAME).
	AdminPublicHostname string `json:"adminPublicHostname,omitempty"`
	// EnginePublicHostname is the public hostname of the PA engine node (PA_ENGINE_PUBLIC_HOSTNAME).
	EnginePublicHostname string `json:"enginePublicHostname,omitempty"`
	// OperationalMode is one of STANDALONE, CLUSTERED_CONSOLE, CLUSTERED_ENGINE (OPERATIONAL_MODE). Default: STANDALONE.
	OperationalMode string `json:"operationalMode,omitempty"`
	// FIPSModeOn enables FIPS mode (FIPS_MODE_ON). Default: false.
	FIPSModeOn bool `json:"fipsModeOn,omitempty"`
	// JavaRAMPercentage is the percentage of container memory for the JVM (JAVA_RAM_PERCENTAGE). Default: 60.0.
	JavaRAMPercentage string `json:"javaRamPercentage,omitempty"`
	// AdminWaitForTimeout is how long (seconds) to wait for the PA admin console before failing (ADMIN_WAITFOR_TIMEOUT). Default: 300.
	AdminWaitForTimeout int32 `json:"adminWaitForTimeout,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials.
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingAccessSpec defines the desired state of a PingAccess deployment.
type PingAccessSpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag or full image reference (e.g. 8.1.0-edge).
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingAccess engine pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// AdminIngress configures the Kubernetes Ingress for the PingAccess admin console.
	// Hostname defaults to pa-admin.<spec.domain> when spec.domain is set.
	AdminIngress IngressSpec `json:"adminIngress,omitempty"`
	// EngineIngress configures the Kubernetes Ingress for the PingAccess engine.
	// Hostname defaults to pa.<spec.domain> when spec.domain is set.
	EngineIngress IngressSpec `json:"engineIngress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	// Annotations are merged on top of spec.services.annotations.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingAccess-specific environment variable configuration.
	Config PingAccessConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingAuthorizeConfig maps to the env vars consumed by the pingauthorize container.
// See: https://developer.pingidentity.com/devops/docker-images/pingauthorize/README.html
type PingAuthorizeConfig struct {
	// ServerProfile is the primary server profile for PingAuthorize.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers (layered profiles).
	// Keys are the layer names (e.g. "PAZ", "BASE"); env vars use the uppercased key.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// LDAPPort is the container LDAP port (LDAP_PORT). Default: 1389.
	LDAPPort int32 `json:"ldapPort,omitempty"`
	// LDAPSPort is the container LDAPS port (LDAPS_PORT). Default: 1636.
	LDAPSPort int32 `json:"ldapsPort,omitempty"`
	// HTTPSPort is the container HTTPS port (HTTPS_PORT). Default: 1443.
	HTTPSPort int32 `json:"httpsPort,omitempty"`
	// UserBaseDN is the base DN for user data (USER_BASE_DN). Default: dc=example,dc=com.
	UserBaseDN string `json:"userBaseDN,omitempty"`
	// AdminUserName is the admin user (ADMIN_USER_NAME). Default: admin.
	AdminUserName string `json:"adminUserName,omitempty"`
	// RetryTimeoutSeconds is the timeout for startup operations (RETRY_TIMEOUT_SECONDS). Default: 180.
	RetryTimeoutSeconds int32 `json:"retryTimeoutSeconds,omitempty"`
	// MaxHeapSize is the JVM maximum heap size (MAX_HEAP_SIZE). Default: 1g.
	MaxHeapSize string `json:"maxHeapSize,omitempty"`
	// AdminSecretRef is the name of a Secret containing the root user password (ROOT_USER_PASSWORD_FILE).
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// EncryptionSecretRef is the name of a Secret containing the encryption passphrase (ENCRYPTION_PASSWORD_FILE).
	EncryptionSecretRef string `json:"encryptionSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingAuthorizePAPConfig maps to the env vars consumed by the pingauthorizepap container.
// See: https://developer.pingidentity.com/devops/docker-images/pingauthorizepap/README.html
type PingAuthorizePAPConfig struct {
	// ServerProfile is the primary server profile for PingAuthorizePAP.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers (layered profiles).
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// ExternalBaseURL is the external hostname and port for PAP API access (PING_EXTERNAL_BASE_URL).
	// Derived as https://paz-pap.<spec.domain> when spec.domain is set and this is empty.
	ExternalBaseURL string `json:"externalBaseURL,omitempty"`
	// OIDCConfigEndpoint is the OIDC provider configuration URL (PING_OIDC_CONFIGURATION_ENDPOINT).
	OIDCConfigEndpoint string `json:"oidcConfigEndpoint,omitempty"`
	// ClientID is the OIDC client identifier (PING_CLIENT_ID).
	ClientID string `json:"clientID,omitempty"`
	// MaxHeapSize is the JVM heap size (MAX_HEAP_SIZE). Default: 384m.
	MaxHeapSize string `json:"maxHeapSize,omitempty"`
	// EnableAPIHTTPCache controls HTTP API caching (PING_ENABLE_API_HTTP_CACHE). Default: true.
	EnableAPIHTTPCache *bool `json:"enableAPIHTTPCache,omitempty"`
	// PolicyDBSync enables database creation/upgrade mode (PING_POLICY_DB_SYNC).
	PolicyDBSync bool `json:"policyDBSync,omitempty"`
	// DBConnectionString is the JDBC connection string for the policy database (PING_DB_CONNECTION_STRING).
	// Use "jdbc:postgresql://<host>:<port>/<db>" for PostgreSQL. Defaults to embedded H2.
	DBConnectionString string `json:"dbConnectionString,omitempty"`
	// DBAdminUsername is the database administrator username for DB creation/upgrade (PING_DB_ADMIN_USERNAME).
	DBAdminUsername string `json:"dbAdminUsername,omitempty"`
	// DBAppUsername is the runtime database username for the Policy Editor (PING_DB_APP_USERNAME).
	DBAppUsername string `json:"dbAppUsername,omitempty"`
	// DBSecretRef is the name of a Secret containing PING_DB_ADMIN_PASSWORD and PING_DB_APP_PASSWORD.
	DBSecretRef string `json:"dbSecretRef,omitempty"`
	// KeystoreFile is the path to the keystore file (KEYSTORE_FILE).
	KeystoreFile string `json:"keystoreFile,omitempty"`
	// KeystorePinFile is the path to the keystore PIN file (KEYSTORE_PIN_FILE).
	KeystorePinFile string `json:"keystorePinFile,omitempty"`
	// KeystoreType is the format of the keystore (KEYSTORE_TYPE). One of: jks, pkcs12.
	KeystoreType string `json:"keystoreType,omitempty"`
	// KeystoreSecretRef is the name of a Secret containing the keystore file and PIN (KEYSTORE_FILE / KEYSTORE_PIN_FILE).
	KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`
	// SharedSecretRef is the name of a Secret containing DECISION_POINT_SHARED_SECRET for PAZ integration.
	SharedSecretRef string `json:"sharedSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingAuthorizePAPSpec defines the desired state of a PingAuthorize PAP (Policy Editor) deployment.
type PingAuthorizePAPSpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag or full image reference.
	Version string `json:"version,omitempty"`
	// Ingress configures the Kubernetes Ingress for the PAP web UI.
	// Hostname defaults to paz-pap.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	// Annotations are merged on top of spec.services.annotations.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingAuthorizePAP-specific environment variable configuration.
	Config PingAuthorizePAPConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingAuthorizeSpec defines the desired state of a PingAuthorize deployment.
type PingAuthorizeSpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag or full image reference (e.g. 10.3.0.0-edge).
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingAuthorize pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// StorageClass is the storage class used for PersistentVolumeClaims.
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of the /opt/out PVC. Default: 8Gi.
	StorageSize string `json:"storageSize,omitempty"`
	// Ingress configures the Kubernetes Ingress for the PingAuthorize management interface.
	// Hostname defaults to paz.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	// Annotations are merged on top of spec.services.annotations.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingAuthorize-specific environment variable configuration.
	Config PingAuthorizeConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingFederateConfig maps to the env vars consumed by the pingfederate container.
// See: https://developer.pingidentity.com/devops/docker-images/pingfederate/README.html
type PingFederateConfig struct {
	// ServerProfile is the primary server profile for PingFederate.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers (layered profiles).
	// Keys are the layer names; env vars use the uppercased key.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// EnginePort is the HTTPS port for the PingFederate runtime engine (PF_ENGINE_PORT). Default: 9031.
	EnginePort int32 `json:"enginePort,omitempty"`
	// AdminPort is the HTTPS port for the PingFederate admin console/API (PF_ADMIN_PORT). Default: 9999.
	AdminPort int32 `json:"adminPort,omitempty"`
	// EngineDebug enables Java remote debugging on the PingFederate engine (PF_ENGINE_DEBUG). Default: false.
	EngineDebug bool `json:"engineDebug,omitempty"`
	// AdminDebug enables Java remote debugging on the PingFederate admin (PF_ADMIN_DEBUG). Default: false.
	AdminDebug bool `json:"adminDebug,omitempty"`
	// DebugPort is the Java remote debug listener port (PF_DEBUG_PORT). Default: 9030.
	// Only meaningful when engineDebug or adminDebug is true.
	DebugPort int32 `json:"debugPort,omitempty"`
	// EnginePublicHostname is the public hostname of the PF engine node (PF_ENGINE_PUBLIC_HOSTNAME).
	EnginePublicHostname string `json:"enginePublicHostname,omitempty"`
	// AdminPublicHostname is the public hostname of the PF admin node (PF_ADMIN_PUBLIC_HOSTNAME).
	AdminPublicHostname string `json:"adminPublicHostname,omitempty"`
	// AdminPublicBaseURL is the full base URL of the PF admin console (PF_ADMIN_PUBLIC_BASEURL).
	AdminPublicBaseURL string `json:"adminPublicBaseURL,omitempty"`
	// ConsoleEnvironment is the environment label shown in the admin console UI (PF_CONSOLE_ENV).
	ConsoleEnvironment string `json:"consoleEnvironment,omitempty"`
	// ConsoleTitle is the title shown in the admin console (PF_CONSOLE_TITLE).
	ConsoleTitle string `json:"consoleTitle,omitempty"`
	// OperationalMode is one of STANDALONE, CLUSTERED_CONSOLE, CLUSTERED_ENGINE (OPERATIONAL_MODE). Default: STANDALONE.
	OperationalMode string `json:"operationalMode,omitempty"`
	// ConsoleAuthentication is the admin console auth mechanism (PF_CONSOLE_AUTHENTICATION). Default: native.
	ConsoleAuthentication string `json:"consoleAuthentication,omitempty"`
	// AdminAPIAuthentication is the admin API auth mechanism (PF_ADMIN_API_AUTHENTICATION). Default: native.
	AdminAPIAuthentication string `json:"adminAPIAuthentication,omitempty"`
	// LDAPType is the LDAP directory type when using LDAP auth (PF_LDAP_TYPE). Default: PingDirectory.
	LDAPType string `json:"ldapType,omitempty"`
	// LDAPUsername is the username for LDAP lookups (PF_LDAP_USERNAME).
	LDAPUsername string `json:"ldapUsername,omitempty"`
	// PingOneRegion is the region of the PingOne tenant (PF_PINGONE_REGION).
	PingOneRegion string `json:"pingOneRegion,omitempty"`
	// PingOneEnvID is the PingOne environment ID to connect to (PF_PINGONE_ENV_ID).
	PingOneEnvID string `json:"pingOneEnvID,omitempty"`
	// ProvisionerMode is one of OFF, STANDALONE, FAILOVER (PF_PROVISIONER_MODE). Default: OFF.
	ProvisionerMode string `json:"provisionerMode,omitempty"`
	// ProvisionerNodeID is the provisioner node ID (PF_PROVISIONER_NODE_ID). Default: 1.
	ProvisionerNodeID int32 `json:"provisionerNodeID,omitempty"`
	// JavaRAMPercentage is the percentage of container memory for the JVM (JAVA_RAM_PERCENTAGE). Default: 75.0.
	JavaRAMPercentage string `json:"javaRamPercentage,omitempty"`
	// HSMMode is the Hardware Security Module mode (HSM_MODE). Default: OFF.
	HSMMode string `json:"hsmMode,omitempty"`
	// HSMHybrid enables HSM hybrid mode where certs/keys can be created on the local trust store or the HSM (PF_HSM_HYBRID). Default: false.
	HSMHybrid bool `json:"hsmHybrid,omitempty"`
	// BCFIPSApprovedOnly restricts the JVM to FIPS-approved algorithms only (PF_BC_FIPS_APPROVED_ONLY). Default: false.
	BCFIPSApprovedOnly bool `json:"bcFIPSApprovedOnly,omitempty"`
	// EngineSecondaryPort is a secondary HTTPS port for mutual SSL/TLS client certificate authentication (PF_ENGINE_SECONDARY_PORT). Default: -1 (disabled).
	EngineSecondaryPort int32 `json:"engineSecondaryPort,omitempty"`
	// NodeTags are space-separated tags associated with this PingFederate node used for request routing (PF_NODE_TAGS).
	NodeTags string `json:"nodeTags,omitempty"`
	// ProvisionerGracePeriod is the provisioner failover grace period in seconds (PF_PROVISIONER_GRACE_PERIOD). Default: 600.
	// Only meaningful when provisionerMode is FAILOVER.
	ProvisionerGracePeriod int32 `json:"provisionerGracePeriod,omitempty"`
	// AdminWaitForTimeout is how long (seconds) to wait for the admin API before failing (ADMIN_WAITFOR_TIMEOUT). Default: 300.
	AdminWaitForTimeout int32 `json:"adminWaitForTimeout,omitempty"`
	// LogSizeMax is the maximum size for all log file appenders (PF_LOG_SIZE_MAX). Default: "10000 KB".
	LogSizeMax string `json:"logSizeMax,omitempty"`
	// LogNumber is the maximum number of log files retained on rotation (PF_LOG_NUMBER). Default: 2.
	LogNumber int32 `json:"logNumber,omitempty"`
	// JettyThreadsMin overrides the minimum Jetty thread pool size (PF_JETTY_THREADS_MIN).
	JettyThreadsMin int32 `json:"jettyThreadsMin,omitempty"`
	// JettyThreadsMax overrides the maximum Jetty thread pool size (PF_JETTY_THREADS_MAX).
	JettyThreadsMax int32 `json:"jettyThreadsMax,omitempty"`
	// AcceptQueueSize sets the size of the Jetty accept queue (PF_ACCEPT_QUEUE_SIZE). Default: 512.
	AcceptQueueSize int32 `json:"acceptQueueSize,omitempty"`
	// CreateInitialAdminUser creates the initial administrator account after first startup (CREATE_INITIAL_ADMIN_USER). Default: false.
	CreateInitialAdminUser bool `json:"createInitialAdminUser,omitempty"`
	// EnableAutomaticHeapDump enables JVM heap dumps on OutOfMemoryError (ENABLE_AUTOMATIC_HEAP_DUMP). Default: true.
	// Set to false to disable automatic heap dumps.
	EnableAutomaticHeapDump *bool `json:"enableAutomaticHeapDump,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials.
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// LDAPSecretRef is the name of a Secret containing PF_LDAP_PASSWORD.
	LDAPSecretRef string `json:"ldapSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingDirectoryConfig maps to the env vars consumed by the pingdirectory container.
// See: https://developer.pingidentity.com/devops/docker-images/pingdirectory/README.html
type PingDirectoryConfig struct {
	// ServerProfile is the primary server profile for PingDirectory.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers (layered profiles).
	// Keys are the layer names; env vars use the uppercased key.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// UserBaseDN is the base DN for user data (USER_BASE_DN). Default: dc=example,dc=com.
	UserBaseDN string `json:"userBaseDN,omitempty"`
	// ReplicationBaseDNs is additional base DNs for replication (REPLICATION_BASE_DNS).
	ReplicationBaseDNs string `json:"replicationBaseDNs,omitempty"`
	// ReplicationPort is the port used for replication communication (REPLICATION_PORT). Default: 8989.
	ReplicationPort int32 `json:"replicationPort,omitempty"`
	// LDAPPort is the container LDAP port (LDAP_PORT). Default: 1389.
	LDAPPort int32 `json:"ldapPort,omitempty"`
	// LDAPSPort is the container LDAPS port (LDAPS_PORT). Default: 1636.
	LDAPSPort int32 `json:"ldapsPort,omitempty"`
	// HTTPSPort is the container HTTPS port (HTTPS_PORT). Default: 1443.
	HTTPSPort int32 `json:"httpsPort,omitempty"`
	// AdminUserName is the replication admin user (ADMIN_USER_NAME). Default: admin.
	AdminUserName string `json:"adminUserName,omitempty"`
	// MakeLdifUsers is the number of users to auto-generate on first start (MAKELDIF_USERS). Default: 0.
	MakeLdifUsers int32 `json:"makeLdifUsers,omitempty"`
	// RetryTimeoutSeconds is the timeout for dsreplication operations (RETRY_TIMEOUT_SECONDS). Default: 180.
	RetryTimeoutSeconds int32 `json:"retryTimeoutSeconds,omitempty"`
	// FIPSModeOn enables Bouncy Castle FIPS mode (FIPS_MODE_ON). Default: false.
	FIPSModeOn bool `json:"fipsModeOn,omitempty"`
	// RebuildOnRestart forces replace-profile on every restart (PD_REBUILD_ON_RESTART). Default: false.
	RebuildOnRestart bool `json:"rebuildOnRestart,omitempty"`
	// ParallelPodManagement must be true when StatefulSet uses Parallel podManagementPolicy. Default: false.
	ParallelPodManagement bool `json:"parallelPodManagement,omitempty"`
	// FailOnDisabledBaseDN fails the container if USER_BASE_DN replication is not enabled (FAIL_ON_DISABLED_BASE_DN). Default: false.
	FailOnDisabledBaseDN bool `json:"failOnDisabledBaseDN,omitempty"`
	// FailOnUnsuccessfulRemoveDefunct fails the container if a previous remove-defunct-server call did not complete successfully (FAIL_ON_UNSUCCESSFUL_REMOVE_DEFUNCT). Default: false.
	FailOnUnsuccessfulRemoveDefunct bool `json:"failOnUnsuccessfulRemoveDefunct,omitempty"`
	// ForceDataReimport forces a backend data export and re-import on restart (PD_FORCE_DATA_REIMPORT). Default: false.
	ForceDataReimport bool `json:"forceDataReimport,omitempty"`
	// SkipWaitForDNS skips the DNS readiness check on startup (SKIP_WAIT_FOR_DNS). Default: false.
	SkipWaitForDNS bool `json:"skipWaitForDNS,omitempty"`
	// LoadBalancingAlgorithmNames is a semicolon-separated list of load-balancing algorithm names (LOAD_BALANCING_ALGORITHM_NAMES).
	LoadBalancingAlgorithmNames string `json:"loadBalancingAlgorithmNames,omitempty"`
	// RestrictedBaseDNs is a semicolon-separated list of base DNs used for entry-balancing configuration (RESTRICTED_BASE_DNS).
	RestrictedBaseDNs string `json:"restrictedBaseDNs,omitempty"`
	// CertificateNickname is the alias of the certificate to use within the keystore (CERTIFICATE_NICKNAME).
	CertificateNickname string `json:"certificateNickname,omitempty"`
	// KeystoreFile is the path to the keystore file (KEYSTORE_FILE).
	KeystoreFile string `json:"keystoreFile,omitempty"`
	// KeystorePinFile is the path to the keystore PIN file (KEYSTORE_PIN_FILE).
	KeystorePinFile string `json:"keystorePinFile,omitempty"`
	// KeystoreType is the format of the keystore (KEYSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	KeystoreType string `json:"keystoreType,omitempty"`
	// TruststoreFile is the path to the truststore file (TRUSTSTORE_FILE).
	TruststoreFile string `json:"truststoreFile,omitempty"`
	// TruststorePinFile is the path to the truststore PIN file (TRUSTSTORE_PIN_FILE).
	TruststorePinFile string `json:"truststorePinFile,omitempty"`
	// TruststoreType is the format of the truststore (TRUSTSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	TruststoreType string `json:"truststoreType,omitempty"`
	// EncryptionSecretRef is the name of a Secret containing the encryption passphrase (ENCRYPTION_PASSWORD_FILE).
	EncryptionSecretRef string `json:"encryptionSecretRef,omitempty"`
	// KeystoreSecretRef is the name of a Secret containing the keystore file and PIN (KEYSTORE_FILE / KEYSTORE_PIN_FILE).
	KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`
	// TruststoreSecretRef is the name of a Secret containing the truststore file and PIN (TRUSTSTORE_FILE / TRUSTSTORE_PIN_FILE).
	TruststoreSecretRef string `json:"truststoreSecretRef,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials.
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingFederateSpec defines the desired state of a PingFederate deployment.
type PingFederateSpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository (e.g. registry.example.com/org/pingfederate).
	// Omit to use the chart's default image repository.
	Image string `json:"image,omitempty"`
	// Version is the container image tag (e.g. 13.0.2-edge).
	// Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingFederate engine pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// EngineIngress configures the Kubernetes Ingress for the PingFederate runtime engine.
	// Hostname defaults to pf.<spec.domain> when spec.domain is set.
	EngineIngress IngressSpec `json:"engineIngress,omitempty"`
	// AdminIngress configures the Kubernetes Ingress for the PingFederate admin console.
	// Hostname defaults to pf-admin.<spec.domain> when spec.domain is set.
	AdminIngress IngressSpec `json:"adminIngress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	// Annotations are merged on top of spec.services.annotations.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingFederate-specific environment variable configuration.
	Config PingFederateConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingDataConsoleSpec configures the PingDataConsole web UI deployment.
// PingDataConsole is only deployed when this section is explicitly present in the spec.
type PingDataConsoleSpec struct {
	// Enabled controls whether PingDataConsole is deployed.
	// Defaults to true when this section is present; set to false to disable.
	Enabled *bool `json:"enabled,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag. Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// HTTPPort is the HTTP listen port for PingDataConsole (HTTP_PORT). Default: 8080.
	HTTPPort int32 `json:"httpPort,omitempty"`
	// HTTPSPort is the HTTPS listen port for PingDataConsole (HTTPS_PORT). Default: 8443.
	HTTPSPort int32 `json:"httpsPort,omitempty"`
	// BrandingAppName sets the application name shown on the sign-on page and banner (BRANDING_APP_NAME).
	// Default: "PingDirectory Admin Console".
	BrandingAppName string `json:"brandingAppName,omitempty"`
	// SystemReadOnly puts the console in read-only mode when true (SYSTEM_READ_ONLY). Default: false.
	SystemReadOnly bool `json:"systemReadOnly,omitempty"`
	// Ingress configures the Kubernetes Ingress for PingDataConsole.
	// Hostname defaults to pd-console.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
}

// PingDirectorySpec defines the desired state of a PingDirectory deployment.
type PingDirectorySpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository (e.g. registry.example.com/org/pingdirectory).
	// Omit to use the chart's default image repository.
	Image string `json:"image,omitempty"`
	// Version is the container image tag (e.g. 11.0.0.2-edge).
	// Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingDirectory pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// StorageClass is the storage class used for PersistentVolumeClaims.
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of the /opt/out PVC. Default: 8Gi.
	StorageSize string `json:"storageSize,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	// Annotations are merged on top of spec.services.annotations.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingDirectory-specific environment variable configuration.
	Config PingDirectoryConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingDataSyncConfig maps to the env vars consumed by the pingdatasync container.
// See: https://developer.pingidentity.com/devops/docker-images/pingdatasync/README.html
type PingDataSyncConfig struct {
	// ServerProfile is the primary server profile for PingDataSync.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// AdminUserName is the failover administrative user (ADMIN_USER_NAME). Default: admin.
	AdminUserName string `json:"adminUserName,omitempty"`
	// RetryTimeoutSeconds is the timeout for manage-topology operations (RETRY_TIMEOUT_SECONDS). Default: 180.
	RetryTimeoutSeconds int32 `json:"retryTimeoutSeconds,omitempty"`
	// RebuildOnRestart forces replace-profile on every restart (PD_REBUILD_ON_RESTART). Default: false.
	RebuildOnRestart bool `json:"rebuildOnRestart,omitempty"`
	// ParallelPodManagement must be true when StatefulSet uses Parallel podManagementPolicy (PARALLEL_POD_MANAGEMENT_POLICY). Default: false.
	ParallelPodManagement bool `json:"parallelPodManagement,omitempty"`
	// SkipWaitForDNS skips the DNS readiness check on startup (SKIP_WAIT_FOR_DNS). Default: false.
	SkipWaitForDNS bool `json:"skipWaitForDNS,omitempty"`
	// CertificateNickname is the alias of the certificate to use within the keystore (CERTIFICATE_NICKNAME).
	CertificateNickname string `json:"certificateNickname,omitempty"`
	// KeystoreFile is the path to the keystore file (KEYSTORE_FILE).
	KeystoreFile string `json:"keystoreFile,omitempty"`
	// KeystorePinFile is the path to the keystore PIN file (KEYSTORE_PIN_FILE).
	KeystorePinFile string `json:"keystorePinFile,omitempty"`
	// KeystoreType is the format of the keystore (KEYSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	KeystoreType string `json:"keystoreType,omitempty"`
	// TruststoreFile is the path to the truststore file (TRUSTSTORE_FILE).
	TruststoreFile string `json:"truststoreFile,omitempty"`
	// TruststorePinFile is the path to the truststore PIN file (TRUSTSTORE_PIN_FILE).
	TruststorePinFile string `json:"truststorePinFile,omitempty"`
	// TruststoreType is the format of the truststore (TRUSTSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	TruststoreType string `json:"truststoreType,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials (ROOT_USER_PASSWORD_FILE / ADMIN_USER_PASSWORD_FILE).
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// KeystoreSecretRef is the name of a Secret containing the keystore file and PIN (KEYSTORE_FILE / KEYSTORE_PIN_FILE).
	KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`
	// TruststoreSecretRef is the name of a Secret containing the truststore file and PIN (TRUSTSTORE_FILE / TRUSTSTORE_PIN_FILE).
	TruststoreSecretRef string `json:"truststoreSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingDataSyncSpec defines the desired state of a PingDataSync deployment.
type PingDataSyncSpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag. Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingDataSync pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// StorageClass is the storage class used for PersistentVolumeClaims.
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of the /opt/out PVC. Default: 8Gi.
	StorageSize string `json:"storageSize,omitempty"`
	// Ingress configures the Kubernetes Ingress for PingDataSync.
	// Hostname defaults to pds.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingDataSync-specific environment variable configuration.
	Config PingDataSyncConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingDirectoryProxyConfig maps to the env vars consumed by the pingdirectoryproxy container.
// See: https://developer.pingidentity.com/devops/docker-images/pingdirectoryproxy/README.html
type PingDirectoryProxyConfig struct {
	// ServerProfile is the primary server profile for PingDirectoryProxy.
	ServerProfile *ServerProfileSpec `json:"serverProfile,omitempty"`
	// ServerProfileLayers defines additional named profile layers.
	ServerProfileLayers []ServerProfileLayerSpec `json:"serverProfileLayers,omitempty"`
	// AdminUserName is the replication administrative user (ADMIN_USER_NAME). Default: admin.
	AdminUserName string `json:"adminUserName,omitempty"`
	// RetryTimeoutSeconds is the timeout for manage-topology operations (RETRY_TIMEOUT_SECONDS). Default: 180.
	RetryTimeoutSeconds int32 `json:"retryTimeoutSeconds,omitempty"`
	// CertificateNickname is the alias of the certificate to use within the keystore (CERTIFICATE_NICKNAME).
	CertificateNickname string `json:"certificateNickname,omitempty"`
	// KeystoreFile is the path to the keystore file (KEYSTORE_FILE).
	KeystoreFile string `json:"keystoreFile,omitempty"`
	// KeystorePinFile is the path to the keystore PIN file (KEYSTORE_PIN_FILE).
	KeystorePinFile string `json:"keystorePinFile,omitempty"`
	// KeystoreType is the format of the keystore (KEYSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	KeystoreType string `json:"keystoreType,omitempty"`
	// TruststoreFile is the path to the truststore file (TRUSTSTORE_FILE).
	TruststoreFile string `json:"truststoreFile,omitempty"`
	// TruststorePinFile is the path to the truststore PIN file (TRUSTSTORE_PIN_FILE).
	TruststorePinFile string `json:"truststorePinFile,omitempty"`
	// TruststoreType is the format of the truststore (TRUSTSTORE_TYPE). One of: jks, pkcs12, pem, bcfks.
	TruststoreType string `json:"truststoreType,omitempty"`
	// PingDirectoryHostname is the PingDirectory hostname used for automatic server discovery (PINGDIRECTORY_HOSTNAME).
	PingDirectoryHostname string `json:"pingDirectoryHostname,omitempty"`
	// PingDirectoryLDAPSPort is the PingDirectory LDAPS port for automatic server discovery (PINGDIRECTORY_LDAPS_PORT).
	PingDirectoryLDAPSPort int32 `json:"pingDirectoryLDAPSPort,omitempty"`
	// JoinPDTopology configures the proxy to join the topology of PingDirectory (JOIN_PD_TOPOLOGY). Default: false.
	JoinPDTopology bool `json:"joinPDTopology,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials (ROOT_USER_PASSWORD_FILE).
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// KeystoreSecretRef is the name of a Secret containing the keystore file and PIN (KEYSTORE_FILE / KEYSTORE_PIN_FILE).
	KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`
	// TruststoreSecretRef is the name of a Secret containing the truststore file and PIN (TRUSTSTORE_FILE / TRUSTSTORE_PIN_FILE).
	TruststoreSecretRef string `json:"truststoreSecretRef,omitempty"`
	// Envs is a map of additional environment variables to inject into this product's container.
	// Keys override any built-in env vars with the same name.
	Envs map[string]string `json:"envs,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingDirectoryProxySpec defines the desired state of a PingDirectoryProxy deployment.
type PingDirectoryProxySpec struct {
	// EnvironmentRef is the name of the PingEnvironment CR this product belongs to.
	EnvironmentRef string `json:"environmentRef,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag. Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingDirectoryProxy pods. Default: 1.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`
	// StorageClass is the storage class used for PersistentVolumeClaims.
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of the /opt/out PVC. Default: 8Gi.
	StorageSize string `json:"storageSize,omitempty"`
	// Ingress configures the Kubernetes Ingress for PingDirectoryProxy.
	// Hostname defaults to pdp.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
	// Service holds Kubernetes Service customisation for this product.
	Service ServiceSpec `json:"service,omitempty"`
	// Container holds container-level settings such as service wait conditions.
	Container ContainerSpec `json:"container,omitempty"`
	// Config holds PingDirectoryProxy-specific environment variable configuration.
	Config PingDirectoryProxyConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingEnvironmentSpec defines the desired state of PingEnvironment.
type PingEnvironmentSpec struct {
	// TenantID is the PingOne tenant identifier used to name Helm releases.
	// It must be a valid DNS-1123 label fragment: the operator derives release,
	// service, and TLS secret names from it (e.g. <tenantId>-ping).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=30
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	TenantID string `json:"tenantId"`
	// Tier is one of: development, staging, production.
	// +kubebuilder:validation:Enum=development;staging;production
	Tier string `json:"tier"`
	// Domain is the base DNS domain for this environment. Component hostnames are derived
	// from this value when not explicitly set (e.g. pf.<domain>, pf-admin.<domain>).
	Domain string `json:"domain,omitempty"`
	// Ingress holds shared ingress settings inherited by all components.
	// Set enabled: true to turn on ingress for all components at once.
	Ingress GlobalIngressSpec `json:"ingress,omitempty"`
	// Services holds shared Kubernetes Service settings inherited by all components.
	// Per-product service.annotations take precedence on conflict.
	Services GlobalServicesSpec `json:"services,omitempty"`
	// PingDataConsole configures the PingDataConsole web UI.
	// Only deployed when this section is explicitly present and a PingDirectory CR references this environment.
	PingDataConsole *PingDataConsoleSpec `json:"pingDataConsole,omitempty"`
	// TargetNamespace is the namespace to deploy into; defaults to metadata.namespace.
	TargetNamespace string `json:"targetNamespace,omitempty"`
	// Vault configures HashiCorp Vault Agent injection for all products in this environment.
	// Maps to global.vault in the ping-devops Helm chart.
	// +kubebuilder:pruning:PreserveUnknownFields
	Vault *runtime.RawExtension `json:"vault,omitempty"`
	// SecurityContext sets the pod-level securityContext for all product workloads.
	// Per-product container.securityContext overrides this.
	// +kubebuilder:pruning:PreserveUnknownFields
	SecurityContext *runtime.RawExtension `json:"securityContext,omitempty"`
	// ContainerSecurityContext sets the container-level securityContext for all product workloads.
	// Per-product container.containerSecurityContext overrides this.
	// +kubebuilder:pruning:PreserveUnknownFields
	ContainerSecurityContext *runtime.RawExtension `json:"containerSecurityContext,omitempty"`
	// Resources sets default CPU/memory requests and limits for all product containers.
	// Per-product container.resources overrides this. Both are overridden by tier defaults
	// when neither is set.
	Resources *ResourceRequirementsSpec `json:"resources,omitempty"`
	// Volumes defines named pod-level volumes available to all product workloads in this
	// environment. Each key is the volume name; the value is any valid Kubernetes volume spec
	// (emptyDir, secret, configMap, hostPath, etc.). Products opt in by listing names under
	// container.includeVolumes, or use IncludeVolumes to mount them in every product.
	// +kubebuilder:pruning:PreserveUnknownFields
	Volumes runtime.RawExtension `json:"volumes,omitempty"`
	// IncludeVolumes lists volume names (from spec.volumes) to mount into every product's
	// workload in this environment. Maps to global.includeVolumes in the ping-devops chart.
	IncludeVolumes []string `json:"includeVolumes,omitempty"`
	// Envs is a map of additional environment variables injected into every product container
	// in this environment. Maps to global.envs in the ping-devops Helm chart.
	Envs map[string]string `json:"envs,omitempty"`
}

// PingEnvironmentStatus defines the observed state of PingEnvironment.
type PingEnvironmentStatus struct {
	// Phase is the current lifecycle phase: Pending, Deploying, Ready, or Failed.
	Phase string `json:"phase"`
	// Release is the Helm release name managed by this environment.
	Release string `json:"release,omitempty"`
	// Conditions holds the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the reconciler.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tenant",type="string",JSONPath=".spec.tenantId"
// +kubebuilder:printcolumn:name="Tier",type="string",JSONPath=".spec.tier"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PingEnvironment is the Schema for the pingenvironments API.
type PingEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PingEnvironmentSpec   `json:"spec,omitempty"`
	Status PingEnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PingEnvironmentList contains a list of PingEnvironment resources.
type PingEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PingEnvironment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PingEnvironment{}, &PingEnvironmentList{})
}
