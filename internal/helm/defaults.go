package helm

import (
	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// applyDefaults fills zero-value fields with the official Ping Identity defaults.
// It is called before BuildPingValues so that all env var mappings have sensible values.
func applyDefaults(env *pingonev1alpha1.PingEnvironmentSpec, products *ProductSpecs) {
	if products.PingFederate != nil {
		if products.PingFederate.Replicas == 0 {
			products.PingFederate.Replicas = 1
		}
		pf := &products.PingFederate.Config
		if pf.EnginePort == 0 {
			pf.EnginePort = 9031
		}
		if pf.AdminPort == 0 {
			pf.AdminPort = 9999
		}
		if pf.JavaRAMPercentage == "" {
			pf.JavaRAMPercentage = "75.0"
		}
	}

	if products.PingAccess != nil {
		if products.PingAccess.Replicas == 0 {
			products.PingAccess.Replicas = 1
		}
		pa := &products.PingAccess.Config
		if pa.AdminPort == 0 {
			pa.AdminPort = 9000
		}
		if pa.EnginePort == 0 {
			pa.EnginePort = 3000
		}
		if pa.JavaRAMPercentage == "" {
			pa.JavaRAMPercentage = "60.0"
		}
	}

	if products.PingAuthorize != nil {
		if products.PingAuthorize.Replicas == 0 {
			products.PingAuthorize.Replicas = 1
		}
		paz := &products.PingAuthorize.Config
		if paz.LDAPPort == 0 {
			paz.LDAPPort = 1389
		}
		if paz.LDAPSPort == 0 {
			paz.LDAPSPort = 1636
		}
		if paz.HTTPSPort == 0 {
			paz.HTTPSPort = 1443
		}
		if paz.UserBaseDN == "" {
			paz.UserBaseDN = "dc=example,dc=com"
		}
		if paz.AdminUserName == "" {
			paz.AdminUserName = "admin"
		}
		if paz.RetryTimeoutSeconds == 0 {
			paz.RetryTimeoutSeconds = 180
		}
		if paz.MaxHeapSize == "" {
			paz.MaxHeapSize = "1g"
		}
		if products.PingAuthorize.StorageSize == "" {
			products.PingAuthorize.StorageSize = "8Gi"
		}
	}

	if products.PingAuthorizePAP != nil {
		pap := &products.PingAuthorizePAP.Config
		if pap.MaxHeapSize == "" {
			pap.MaxHeapSize = "384m"
		}
		if pap.EnableAPIHTTPCache == nil {
			t := true
			pap.EnableAPIHTTPCache = &t
		}
	}

	if products.PingDataSync != nil {
		if products.PingDataSync.Replicas == 0 {
			products.PingDataSync.Replicas = 1
		}
		pds := &products.PingDataSync.Config
		if pds.AdminUserName == "" {
			pds.AdminUserName = "admin"
		}
		if pds.RetryTimeoutSeconds == 0 {
			pds.RetryTimeoutSeconds = 180
		}
		if products.PingDataSync.StorageSize == "" {
			products.PingDataSync.StorageSize = "8Gi"
		}
	}

	if products.PingDirectoryProxy != nil {
		if products.PingDirectoryProxy.Replicas == 0 {
			products.PingDirectoryProxy.Replicas = 1
		}
		pdp := &products.PingDirectoryProxy.Config
		if pdp.AdminUserName == "" {
			pdp.AdminUserName = "admin"
		}
		if pdp.RetryTimeoutSeconds == 0 {
			pdp.RetryTimeoutSeconds = 180
		}
		if products.PingDirectoryProxy.StorageSize == "" {
			products.PingDirectoryProxy.StorageSize = "8Gi"
		}
	}

	if products.PingDataConsole != nil {
		if products.PingDataConsole.Replicas == 0 {
			products.PingDataConsole.Replicas = 1
		}
	}

	if products.PingDirectory == nil {
		return
	}

	if products.PingDirectory.Replicas == 0 {
		products.PingDirectory.Replicas = 1
	}

	pd := &products.PingDirectory.Config

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
	if products.PingDirectory.StorageSize == "" {
		products.PingDirectory.StorageSize = "8Gi"
	}
}
