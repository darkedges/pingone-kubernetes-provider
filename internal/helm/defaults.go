package helm

import (
	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// applyDefaults fills zero-value fields in spec with the official Ping Identity defaults.
// It is called before BuildPingValues so that all env var mappings have sensible values.
func applyDefaults(spec *pingonev1alpha1.PingEnvironmentSpec) {
	if spec.PingFederate.Replicas == 0 {
		spec.PingFederate.Replicas = 1
	}

	pf := &spec.PingFederate.Config

	if pf.EnginePort == 0 {
		pf.EnginePort = 9031
	}
	if pf.AdminPort == 0 {
		pf.AdminPort = 9999
	}
	if pf.OperationalMode == "" {
		pf.OperationalMode = "STANDALONE"
	}
	if pf.ConsoleAuthentication == "" {
		pf.ConsoleAuthentication = "native"
	}
	if pf.AdminAPIAuthentication == "" {
		pf.AdminAPIAuthentication = "native"
	}
	if pf.LDAPType == "" {
		pf.LDAPType = "PingDirectory"
	}
	if pf.ProvisionerMode == "" {
		pf.ProvisionerMode = "OFF"
	}
	if pf.ProvisionerNodeID == 0 {
		pf.ProvisionerNodeID = 1
	}
	if pf.JavaRAMPercentage == "" {
		pf.JavaRAMPercentage = "75.0"
	}
	if pf.HSMMode == "" {
		pf.HSMMode = "OFF"
	}

	if spec.PingDirectory == nil {
		return
	}

	if spec.PingDirectory.Replicas == 0 {
		spec.PingDirectory.Replicas = 1
	}

	pd := &spec.PingDirectory.Config

	if pd.UserBaseDN == "" {
		pd.UserBaseDN = "dc=example,dc=com"
	}
	if pd.ReplicationPort == 0 {
		pd.ReplicationPort = 8989
	}
	if pd.LDAPPort == 0 {
		pd.LDAPPort = 1389
	}
	if pd.LDAPSPort == 0 {
		pd.LDAPSPort = 1636
	}
	if pd.HTTPSPort == 0 {
		pd.HTTPSPort = 1443
	}
	if pd.AdminUserName == "" {
		pd.AdminUserName = "admin"
	}
	if pd.RetryTimeoutSeconds == 0 {
		pd.RetryTimeoutSeconds = 180
	}
	if spec.PingDirectory.StorageSize == "" {
		spec.PingDirectory.StorageSize = "8Gi"
	}
}
