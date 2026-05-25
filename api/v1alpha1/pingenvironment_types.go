package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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

// PingFederateConfig maps to the env vars consumed by the pingfederate container.
// See: https://developer.pingidentity.com/devops/docker-images/pingfederate/README.html
type PingFederateConfig struct {
	// ServerProfileURL is the Git HTTPS URL of the server profile repo (SERVER_PROFILE_URL).
	ServerProfileURL string `json:"serverProfileURL,omitempty"`
	// ServerProfileBranch is the Git branch to check out (SERVER_PROFILE_BRANCH).
	ServerProfileBranch string `json:"serverProfileBranch,omitempty"`
	// ServerProfilePath is the subdirectory within the git repo (SERVER_PROFILE_PATH).
	ServerProfilePath string `json:"serverProfilePath,omitempty"`
	// EnginePort is the HTTPS port for the PingFederate runtime engine (PF_ENGINE_PORT). Default: 9031.
	EnginePort int32 `json:"enginePort,omitempty"`
	// AdminPort is the HTTPS port for the PingFederate admin console/API (PF_ADMIN_PORT). Default: 9999.
	AdminPort int32 `json:"adminPort,omitempty"`
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
	// AdminSecretRef is the name of a Secret containing admin credentials.
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// LDAPSecretRef is the name of a Secret containing PF_LDAP_PASSWORD.
	LDAPSecretRef string `json:"ldapSecretRef,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingDirectoryConfig maps to the env vars consumed by the pingdirectory container.
// See: https://developer.pingidentity.com/devops/docker-images/pingdirectory/README.html
type PingDirectoryConfig struct {
	// ServerProfileURL is the Git HTTPS URL of the server profile repo (SERVER_PROFILE_URL).
	ServerProfileURL string `json:"serverProfileURL,omitempty"`
	// ServerProfileBranch is the Git branch to check out (SERVER_PROFILE_BRANCH).
	ServerProfileBranch string `json:"serverProfileBranch,omitempty"`
	// ServerProfilePath is the subdirectory within the git repo (SERVER_PROFILE_PATH).
	ServerProfilePath string `json:"serverProfilePath,omitempty"`
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
	// FailOnDisabledBaseDN fails the container if USER_BASE_DN replication is not enabled. Default: false.
	FailOnDisabledBaseDN bool `json:"failOnDisabledBaseDN,omitempty"`
	// AdminSecretRef is the name of a Secret containing admin credentials.
	AdminSecretRef string `json:"adminSecretRef,omitempty"`
	// KeystoreSecretRef is the name of a Secret containing the keystore for TLS.
	KeystoreSecretRef string `json:"keystoreSecretRef,omitempty"`
	// EnvConfigMapRef is the name of a ConfigMap with additional env vars to inject.
	EnvConfigMapRef string `json:"envConfigMapRef,omitempty"`
}

// PingFederateSpec defines the desired state of a PingFederate deployment.
type PingFederateSpec struct {
	// Image is the container image repository (e.g. registry.example.com/org/pingfederate).
	// Omit to use the chart's default image repository.
	Image string `json:"image,omitempty"`
	// Version is the container image tag (e.g. 13.0.2-edge).
	// Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingFederate engine pods. Default: 1.
	Replicas int32 `json:"replicas,omitempty"`
	// EngineIngress configures the Kubernetes Ingress for the PingFederate runtime engine.
	// Hostname defaults to pf.<spec.domain> when spec.domain is set.
	EngineIngress IngressSpec `json:"engineIngress,omitempty"`
	// AdminIngress configures the Kubernetes Ingress for the PingFederate admin console.
	// Hostname defaults to pf-admin.<spec.domain> when spec.domain is set.
	AdminIngress IngressSpec `json:"adminIngress,omitempty"`
	// Config holds PingFederate-specific environment variable configuration.
	Config PingFederateConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingDataConsoleSpec configures the PingDataConsole web UI deployment.
// PingDataConsole is automatically enabled when spec.pingDirectory is set.
type PingDataConsoleSpec struct {
	// Enabled controls whether PingDataConsole is deployed.
	// Defaults to true when spec.pingDirectory is configured; set to false to disable.
	Enabled *bool `json:"enabled,omitempty"`
	// Image is the container image repository. Omit to use the chart's default.
	Image string `json:"image,omitempty"`
	// Version is the container image tag. Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Ingress configures the Kubernetes Ingress for PingDataConsole.
	// Hostname defaults to pd-console.<spec.domain> when spec.domain is set.
	Ingress IngressSpec `json:"ingress,omitempty"`
}

// PingDirectorySpec defines the desired state of a PingDirectory deployment.
type PingDirectorySpec struct {
	// Image is the container image repository (e.g. registry.example.com/org/pingdirectory).
	// Omit to use the chart's default image repository.
	Image string `json:"image,omitempty"`
	// Version is the container image tag (e.g. 11.0.0.2-edge).
	// Omit to use the chart's default tag.
	Version string `json:"version,omitempty"`
	// Replicas is the desired number of PingDirectory pods. Default: 1.
	Replicas int32 `json:"replicas,omitempty"`
	// StorageClass is the storage class used for PersistentVolumeClaims.
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of the /opt/out PVC. Default: 8Gi.
	StorageSize string `json:"storageSize,omitempty"`
	// Config holds PingDirectory-specific environment variable configuration.
	Config PingDirectoryConfig `json:"config,omitempty"`
	// ValuesOverride is merged on top of the base Helm values as raw JSON.
	ValuesOverride runtime.RawExtension `json:"valuesOverride,omitempty"`
}

// PingEnvironmentSpec defines the desired state of PingEnvironment.
type PingEnvironmentSpec struct {
	// TenantID is the PingOne tenant identifier used to name Helm releases.
	TenantID string `json:"tenantId"`
	// Tier is one of: development, staging, production.
	Tier string `json:"tier"`
	// Domain is the base DNS domain for this environment. Component hostnames are derived
	// from this value when not explicitly set (e.g. pf.<domain>, pf-admin.<domain>).
	Domain string `json:"domain,omitempty"`
	// Ingress holds shared ingress settings inherited by all components.
	// Set enabled: true to turn on ingress for all components at once.
	Ingress GlobalIngressSpec `json:"ingress,omitempty"`
	// PingFederate holds the PingFederate deployment configuration.
	PingFederate PingFederateSpec `json:"pingFederate"`
	// PingDirectory holds the optional PingDirectory deployment configuration.
	PingDirectory *PingDirectorySpec `json:"pingDirectory,omitempty"`
	// PingDataConsole configures the PingDataConsole web UI.
	// Automatically enabled when pingDirectory is set; set enabled: false to disable.
	PingDataConsole *PingDataConsoleSpec `json:"pingDataConsole,omitempty"`
	// TargetNamespace is the namespace to deploy into; defaults to metadata.namespace.
	TargetNamespace string `json:"targetNamespace,omitempty"`
}

// PingEnvironmentStatus defines the observed state of PingEnvironment.
type PingEnvironmentStatus struct {
	// Phase is the current lifecycle phase: Pending, Deploying, Ready, or Failed.
	Phase string `json:"phase"`
	// Conditions holds the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// PingFederateRelease is the Helm release name for PingFederate.
	PingFederateRelease string `json:"pingFederateRelease,omitempty"`
	// PingDirectoryRelease is the Helm release name for PingDirectory.
	PingDirectoryRelease string `json:"pingDirectoryRelease,omitempty"`
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
