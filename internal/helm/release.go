package helm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// ReleaseExists reports whether a Helm release with the given name is already installed.
func ReleaseExists(cfg *action.Configuration, releaseName string) bool {
	list := action.NewList(cfg)
	list.All = true
	releases, err := list.Run()
	if err != nil {
		return false
	}
	for _, r := range releases {
		if r.Name == releaseName {
			return true
		}
	}
	return false
}

// MergeValues deep-merges override (raw JSON from a RawExtension) on top of base.
// Keys present in override overwrite those in base; nested maps are merged recursively.
func MergeValues(base map[string]any, override runtime.RawExtension) (map[string]any, error) {
	if len(override.Raw) == 0 {
		return base, nil
	}
	var extra map[string]any
	if err := json.Unmarshal(override.Raw, &extra); err != nil {
		return nil, fmt.Errorf("unmarshal valuesOverride: %w", err)
	}
	return mergeMaps(base, extra), nil
}

// InstallOrUpgrade installs a Helm chart for the first time or upgrades an existing release.
// upgrade controls whether an existing release is upgraded; if false and the release exists,
// this is a no-op.
func InstallOrUpgrade(cfg *action.Configuration, releaseName, namespace string, ch *chart.Chart, values map[string]any, upgrade bool) error {
	if ReleaseExists(cfg, releaseName) {
		if !upgrade {
			return nil
		}
		up := action.NewUpgrade(cfg)
		up.ReuseValues = false
		_, err := up.Run(releaseName, ch, values)
		return err
	}
	inst := action.NewInstall(cfg)
	inst.ReleaseName = releaseName
	inst.Namespace = namespace
	inst.CreateNamespace = false
	_, err := inst.Run(ch, values)
	return err
}

// DownloadChart downloads a chart from repoURL into cacheDir and returns the local path.
// Subsequent calls for the same chart version return the cached path without a network fetch.
func DownloadChart(repoURL, chartName, version, cacheDir string) (string, error) {
	cached := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", chartName, version))
	if _, err := os.Stat(cached); err == nil {
		return cached, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	entry := &repo.Entry{Name: "pingidentity", URL: repoURL}
	getters := getter.All(cli.New())
	cr, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return "", fmt.Errorf("chart repository: %w", err)
	}
	cr.CachePath = cacheDir

	idxPath, err := cr.DownloadIndexFile()
	if err != nil {
		return "", fmt.Errorf("download repo index: %w", err)
	}
	idxData, err := os.ReadFile(idxPath)
	if err != nil {
		return "", fmt.Errorf("read index file: %w", err)
	}
	var idx repo.IndexFile
	if err := yaml.Unmarshal(idxData, &idx); err != nil {
		return "", fmt.Errorf("parse index: %w", err)
	}
	idx.SortEntries()
	cv, err := idx.Get(chartName, version)
	if err != nil {
		return "", fmt.Errorf("find chart %s@%s: %w", chartName, version, err)
	}
	if len(cv.URLs) == 0 {
		return "", fmt.Errorf("no download URLs for chart %s@%s", chartName, version)
	}

	chartURL := cv.URLs[0]
	if !strings.HasPrefix(chartURL, "http") {
		chartURL = strings.TrimSuffix(repoURL, "/") + "/" + chartURL
	}

	g, err := getters.ByScheme("https")
	if err != nil {
		return "", fmt.Errorf("https getter: %w", err)
	}
	data, err := g.Get(chartURL)
	if err != nil {
		return "", fmt.Errorf("download chart %s: %w", chartURL, err)
	}

	f, err := os.Create(cached)
	if err != nil {
		return "", fmt.Errorf("create cache file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, data); err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}
	return cached, nil
}

// TierResources returns CPU and memory resource requests for a given tier and product.
// product must be "pf" (PingFederate) or "pd" (PingDirectory).
// Tier values: development, staging, production.
func TierResources(tier, product string) (cpu, memory string) {
	switch product {
	case "pd":
		switch tier {
		case "production":
			return "2", "4Gi"
		case "staging":
			return "1", "2Gi"
		default: // development
			return "500m", "1Gi"
		}
	default: // pf
		switch tier {
		case "production":
			return "2", "2Gi"
		case "staging":
			return "1", "1Gi"
		default: // development
			return "500m", "512Mi"
		}
	}
}

// BuildPingValues constructs the full Helm values map for a ping-devops release
// from a PingEnvironment spec. It applies defaults, builds the global section,
// the pingfederate sub-chart section, and the pingdirectory sub-chart section,
// then merges any ValuesOverride on top.
func BuildPingValues(spec pingonev1alpha1.PingEnvironmentSpec) (map[string]any, error) {
	applyDefaults(&spec)

	pfCPU, pfMem := TierResources(spec.Tier, "pf")
	pfCfg := spec.PingFederate.Config
	pfIng := resolveIngressSpec(spec.Ingress, spec.PingFederate.EngineIngress)
	pfAdminIng := resolveIngressSpec(spec.Ingress, spec.PingFederate.AdminIngress)

	// Resolve hostnames from domain when not explicitly set
	engineHostname := resolveHostname(pfIng.Hostname, "pf", spec.Domain)
	adminHostname := resolveHostname(pfAdminIng.Hostname, "pf-admin", spec.Domain)

	// Build global section
	globalValues := map[string]any{
		"envs": map[string]any{
			"PING_IDENTITY_ACCEPT_EULA": "YES",
		},
		"envFrom": map[string]any{
			"secretRef": []map[string]any{
				{"name": "devops-secret"},
			},
		},
		"ingress": map[string]any{
			"enabled": false,
		},
	}

	// Build PingFederate envs
	pfEnvs := map[string]any{}

	emitServerProfileEnvs(pfEnvs, pfCfg.ServerProfile, pfCfg.ServerProfileLayers)
	pfEnvs["SERVER_PROFILE_UPDATE"] = "false"

	// Ports (always set after defaults applied)
	pfEnvs["PF_ENGINE_PORT"] = fmt.Sprintf("%d", pfCfg.EnginePort)
	pfEnvs["PF_ADMIN_PORT"] = fmt.Sprintf("%d", pfCfg.AdminPort)

	// Hostnames
	if pfCfg.EnginePublicHostname != "" {
		pfEnvs["PF_ENGINE_PUBLIC_HOSTNAME"] = pfCfg.EnginePublicHostname
	}
	if pfCfg.AdminPublicHostname != "" {
		pfEnvs["PF_ADMIN_PUBLIC_HOSTNAME"] = pfCfg.AdminPublicHostname
	}
	if pfCfg.AdminPublicBaseURL != "" {
		pfEnvs["PF_ADMIN_PUBLIC_BASEURL"] = pfCfg.AdminPublicBaseURL
	}

	// Console branding
	if pfCfg.ConsoleEnvironment != "" {
		pfEnvs["PF_CONSOLE_ENV"] = pfCfg.ConsoleEnvironment
	}
	if pfCfg.ConsoleTitle != "" {
		pfEnvs["PF_CONSOLE_TITLE"] = pfCfg.ConsoleTitle
	}

	// Operational mode
	pfEnvs["OPERATIONAL_MODE"] = pfCfg.OperationalMode
	pfEnvs["CLUSTER_BIND_ADDRESS"] = "NON_LOOPBACK"

	// Authentication
	pfEnvs["PF_CONSOLE_AUTHENTICATION"] = pfCfg.ConsoleAuthentication
	pfEnvs["PF_ADMIN_API_AUTHENTICATION"] = pfCfg.AdminAPIAuthentication
	pfEnvs["PF_LDAP_TYPE"] = pfCfg.LDAPType
	if pfCfg.LDAPUsername != "" {
		pfEnvs["PF_LDAP_USERNAME"] = pfCfg.LDAPUsername
	}

	// PingOne integration
	if pfCfg.PingOneRegion != "" {
		pfEnvs["PF_PINGONE_REGION"] = pfCfg.PingOneRegion
	}
	if pfCfg.PingOneEnvID != "" {
		pfEnvs["PF_PINGONE_ENV_ID"] = pfCfg.PingOneEnvID
	}

	// Provisioner
	pfEnvs["PF_PROVISIONER_MODE"] = pfCfg.ProvisionerMode
	pfEnvs["PF_PROVISIONER_NODE_ID"] = fmt.Sprintf("%d", pfCfg.ProvisionerNodeID)
	pfEnvs["PF_PROVISIONER_GRACE_PERIOD"] = "600"

	// JVM
	pfEnvs["JAVA_RAM_PERCENTAGE"] = pfCfg.JavaRAMPercentage

	// HSM
	pfEnvs["HSM_MODE"] = pfCfg.HSMMode

	// Logging
	pfEnvs["TAIL_LOG_FILES"] = "${SERVER_ROOT_DIR}/log/server.log"

	// Build PingFederate envFrom
	pfEnvFrom := map[string]any{}
	if pfCfg.AdminSecretRef != "" || pfCfg.LDAPSecretRef != "" {
		secretRefs := []map[string]any{}
		if pfCfg.AdminSecretRef != "" {
			secretRefs = append(secretRefs, map[string]any{"name": pfCfg.AdminSecretRef})
		}
		if pfCfg.LDAPSecretRef != "" {
			secretRefs = append(secretRefs, map[string]any{"name": pfCfg.LDAPSecretRef})
		}
		pfEnvFrom["secretRef"] = secretRefs
	}
	if pfCfg.EnvConfigMapRef != "" {
		pfEnvFrom["configMapRef"] = []map[string]any{
			{"name": pfCfg.EnvConfigMapRef},
		}
	}

	// Auto-derive TLS secret names when not explicitly set
	pfIng.TLSSecretRef = resolveTLSSecretRef(pfIng.TLSSecretRef, spec.TenantID, "pf")
	pfAdminIng.TLSSecretRef = resolveTLSSecretRef(pfAdminIng.TLSSecretRef, spec.TenantID, "pf-admin")

	// Build PingFederate engine ingress
	var pfEngineIngressValues map[string]any
	if ingressEnabled(pfIng) {
		pfEngineIngressValues = buildIngressValues(pfIng, engineHostname)
	} else {
		pfEngineIngressValues = map[string]any{"enabled": false}
	}

	// Build PingFederate admin ingress
	var pfAdminIngressValues map[string]any
	adminSvc := map[string]any{
		"containerPort": pfCfg.AdminPort,
		"servicePort":   pfCfg.AdminPort,
		"dataService":   true,
	}
	if ingressEnabled(pfAdminIng) {
		pfAdminIngressValues = buildIngressValues(pfAdminIng, adminHostname)
		adminSvc["ingressPort"] = 443
	} else {
		pfAdminIngressValues = map[string]any{"enabled": false}
	}

	pfImageValues := buildImageValues(spec.PingFederate.Image, spec.PingFederate.Version)

	// Assemble pingfederate-admin section (admin console, 1 replica)
	pfAdminValues := map[string]any{
		"enabled": true,
		"workload": map[string]any{
			"type": "Deployment",
			"deployment": map[string]any{
				"replicas": 1,
			},
		},
		"image":     pfImageValues,
		"container": buildContainerValues(pfCPU, pfMem, spec.PingFederate.Container),
		"envs":      pfEnvs,
		"services": map[string]any{
			"https": adminSvc,
		},
		"ingress": pfAdminIngressValues,
	}
	if len(pfEnvFrom) > 0 {
		pfAdminValues["envFrom"] = pfEnvFrom
	}

	// Assemble pingfederate-engine section (runtime engine, user-specified replicas)
	pfEngineValues := map[string]any{
		"enabled": true,
		"workload": map[string]any{
			"type": "Deployment",
			"deployment": map[string]any{
				"replicas": spec.PingFederate.Replicas,
			},
		},
		"image":     pfImageValues,
		"container": buildContainerValues(pfCPU, pfMem, spec.PingFederate.Container),
		"envs": pfEnvs,
		"services": map[string]any{
			"https": map[string]any{
				"containerPort": pfCfg.EnginePort,
				"servicePort":   pfCfg.EnginePort,
				"ingressPort":   443,
				"dataService":   true,
			},
		},
		"ingress": pfEngineIngressValues,
	}
	if len(pfEnvFrom) > 0 {
		pfEngineValues["envFrom"] = pfEnvFrom
	}

	// Assemble pingdirectory section
	var pdValues map[string]any
	if spec.PingDirectory == nil {
		pdValues = map[string]any{"enabled": false}
	} else {
		pdCPU, pdMem := TierResources(spec.Tier, "pd")
		pdCfg := spec.PingDirectory.Config
		pdSpec := spec.PingDirectory

		// Build PingDirectory envs
		pdEnvs := map[string]any{}

		emitServerProfileEnvs(pdEnvs, pdCfg.ServerProfile, pdCfg.ServerProfileLayers)

		pdEnvs["USER_BASE_DN"] = pdCfg.UserBaseDN
		if pdCfg.ReplicationBaseDNs != "" {
			pdEnvs["REPLICATION_BASE_DNS"] = pdCfg.ReplicationBaseDNs
		}
		pdEnvs["REPLICATION_PORT"] = fmt.Sprintf("%d", pdCfg.ReplicationPort)
		pdEnvs["ADMIN_USER_NAME"] = pdCfg.AdminUserName
		pdEnvs["MAKELDIF_USERS"] = fmt.Sprintf("%d", pdCfg.MakeLdifUsers)
		pdEnvs["RETRY_TIMEOUT_SECONDS"] = fmt.Sprintf("%d", pdCfg.RetryTimeoutSeconds)
		pdEnvs["LDAP_PORT"] = fmt.Sprintf("%d", pdCfg.LDAPPort)
		pdEnvs["LDAPS_PORT"] = fmt.Sprintf("%d", pdCfg.LDAPSPort)
		pdEnvs["HTTPS_PORT"] = fmt.Sprintf("%d", pdCfg.HTTPSPort)

		pdEnvs["FIPS_MODE_ON"] = fmt.Sprintf("%t", pdCfg.FIPSModeOn)
		pdEnvs["PD_REBUILD_ON_RESTART"] = fmt.Sprintf("%t", pdCfg.RebuildOnRestart)
		pdEnvs["FAIL_ON_DISABLED_BASE_DN"] = fmt.Sprintf("%t", pdCfg.FailOnDisabledBaseDN)
		pdEnvs["PARALLEL_POD_MANAGEMENT_POLICY"] = fmt.Sprintf("%t", pdCfg.ParallelPodManagement)
		pdEnvs["UNBOUNDID_SKIP_START_PRECHECK_NODETACH"] = "true"
		pdEnvs["JAVA_RAM_PERCENTAGE"] = "75.0"
		pdEnvs["TAIL_LOG_FILES"] = "${SERVER_ROOT_DIR}/logs/access ${SERVER_ROOT_DIR}/logs/errors ${SERVER_ROOT_DIR}/logs/failed-ops ${SERVER_ROOT_DIR}/logs/config-audit.log"

		// Pod management policy
		podMgmtPolicy := "OrderedReady"
		if pdCfg.ParallelPodManagement {
			podMgmtPolicy = "Parallel"
		}

		// PVC storage class
		pvcClaim := map[string]any{
			"accessModes": []string{"ReadWriteOnce"},
			"resources": map[string]any{
				"requests": map[string]any{
					"storage": pdSpec.StorageSize,
				},
			},
		}
		if pdSpec.StorageClass != "" {
			pvcClaim["storageClassName"] = pdSpec.StorageClass
		}

		// PingDirectory envFrom
		pdEnvFrom := map[string]any{}
		if pdCfg.AdminSecretRef != "" {
			pdEnvFrom["secretRef"] = []map[string]any{
				{"name": pdCfg.AdminSecretRef},
			}
		}
		if pdCfg.EnvConfigMapRef != "" {
			pdEnvFrom["configMapRef"] = []map[string]any{
				{"name": pdCfg.EnvConfigMapRef},
			}
		}

		pdValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "StatefulSet",
				"statefulSet": map[string]any{
					"replicas":            pdSpec.Replicas,
					"podManagementPolicy": podMgmtPolicy,
					"persistentvolume": map[string]any{
						"enabled": true,
						"volumes": map[string]any{
							"out-dir": map[string]any{
								"mountPath":             "/opt/out",
								"persistentVolumeClaim": pvcClaim,
							},
						},
					},
				},
			},
			"image":     buildImageValues(pdSpec.Image, pdSpec.Version),
			"container": buildContainerValues(pdCPU, pdMem, pdSpec.Container),
			"envs": pdEnvs,
			"services": map[string]any{
				"ldap": map[string]any{
					"containerPort":  pdCfg.LDAPPort,
					"servicePort":    pdCfg.LDAPPort,
					"clusterService": true,
				},
				"ldaps": map[string]any{
					"containerPort":  pdCfg.LDAPSPort,
					"servicePort":    pdCfg.LDAPSPort,
					"clusterService": true,
				},
				"https": map[string]any{
					"containerPort": pdCfg.HTTPSPort,
					"servicePort":   pdCfg.HTTPSPort,
					"dataService":   true,
				},
			},
			"ingress": map[string]any{"enabled": false},
		}
		if len(pdEnvFrom) > 0 {
			pdValues["envFrom"] = pdEnvFrom
		}
	}

	// Assemble pingdataconsole section.
	// Enabled by default when pingDirectory is set; disabled via spec.pingDataConsole.enabled=false.
	var pdcValues map[string]any
	pdcShouldEnable := spec.PingDirectory != nil &&
		(spec.PingDataConsole == nil ||
			spec.PingDataConsole.Enabled == nil ||
			*spec.PingDataConsole.Enabled)
	if pdcShouldEnable {
		var pdc pingonev1alpha1.PingDataConsoleSpec
		if spec.PingDataConsole != nil {
			pdc = *spec.PingDataConsole
		}
		pdcIng := resolveIngressSpec(spec.Ingress, pdc.Ingress)
		pdcIng.TLSSecretRef = resolveTLSSecretRef(pdcIng.TLSSecretRef, spec.TenantID, "pd-console")
		pdcHostname := resolveHostname(pdcIng.Hostname, "pd-console", spec.Domain)

		var pdcIngressValues map[string]any
		if ingressEnabled(pdcIng) {
			pdcIngressValues = buildIngressValues(pdcIng, pdcHostname)
		} else {
			pdcIngressValues = map[string]any{"enabled": false}
		}

		// Cluster service name for PingDirectory: <tenantId>-ping-pingdirectory-cluster
		pdClusterSvc := fmt.Sprintf("%s-ping-pingdirectory-cluster", spec.TenantID)

		// Console image: use explicit image/version, fall back to PingDirectory's values
		pdcImage := buildImageValues(
			firstNonEmpty(pdc.Image, spec.PingDirectory.Image),
			firstNonEmpty(pdc.Version, spec.PingDirectory.Version),
		)

		pdcValues = map[string]any{
			"enabled": true,
			"image":   pdcImage,
			"defaultLogin": map[string]any{
				"server": map[string]any{
					"host": pdClusterSvc,
					"port": spec.PingDirectory.Config.LDAPSPort,
				},
			},
			"ingress": pdcIngressValues,
		}
	} else {
		pdcValues = map[string]any{"enabled": false}
	}

	// Assemble pingaccess-admin and pingaccess-engine sections
	var paAdminValues, paEngineValues map[string]any
	if spec.PingAccess == nil {
		paAdminValues = map[string]any{"enabled": false}
		paEngineValues = map[string]any{"enabled": false}
	} else {
		paCPU, paMem := TierResources(spec.Tier, "pf")
		paCfg := spec.PingAccess.Config
		paAdminIng := resolveIngressSpec(spec.Ingress, spec.PingAccess.AdminIngress)
		paEngineIng := resolveIngressSpec(spec.Ingress, spec.PingAccess.EngineIngress)
		paAdminIng.TLSSecretRef = resolveTLSSecretRef(paAdminIng.TLSSecretRef, spec.TenantID, "pa-admin")
		paEngineIng.TLSSecretRef = resolveTLSSecretRef(paEngineIng.TLSSecretRef, spec.TenantID, "pa")
		paAdminHostname := resolveHostname(paAdminIng.Hostname, "pa-admin", spec.Domain)
		paEngineHostname := resolveHostname(paEngineIng.Hostname, "pa", spec.Domain)

		paEnvs := map[string]any{
			"PA_ADMIN_PORT":       fmt.Sprintf("%d", paCfg.AdminPort),
			"PA_ENGINE_PORT":      fmt.Sprintf("%d", paCfg.EnginePort),
			"OPERATIONAL_MODE":    paCfg.OperationalMode,
			"FIPS_MODE_ON":        fmt.Sprintf("%t", paCfg.FIPSModeOn),
			"JAVA_RAM_PERCENTAGE": paCfg.JavaRAMPercentage,
			"TAIL_LOG_FILES":      "${SERVER_ROOT_DIR}/log/pingaccess.log",
		}
		emitServerProfileEnvs(paEnvs, paCfg.ServerProfile, paCfg.ServerProfileLayers)
		if paCfg.AdminPublicHostname != "" {
			paEnvs["PA_ADMIN_PUBLIC_HOSTNAME"] = paCfg.AdminPublicHostname
		}
		if paCfg.EnginePublicHostname != "" {
			paEnvs["PA_ENGINE_PUBLIC_HOSTNAME"] = paCfg.EnginePublicHostname
		}

		paEnvFrom := map[string]any{}
		if paCfg.AdminSecretRef != "" {
			paEnvFrom["secretRef"] = []map[string]any{{"name": paCfg.AdminSecretRef}}
		}
		if paCfg.EnvConfigMapRef != "" {
			paEnvFrom["configMapRef"] = []map[string]any{{"name": paCfg.EnvConfigMapRef}}
		}

		var paAdminIngressValues map[string]any
		if ingressEnabled(paAdminIng) {
			paAdminIngressValues = buildIngressValues(paAdminIng, paAdminHostname)
		} else {
			paAdminIngressValues = map[string]any{"enabled": false}
		}

		var paEngineIngressValues map[string]any
		if ingressEnabled(paEngineIng) {
			paEngineIngressValues = buildIngressValues(paEngineIng, paEngineHostname)
		} else {
			paEngineIngressValues = map[string]any{"enabled": false}
		}

		paImageValues := buildImageValues(spec.PingAccess.Image, spec.PingAccess.Version)

		paAdminValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": 1,
				},
			},
			"image":     paImageValues,
			"container": buildContainerValues(paCPU, paMem, spec.PingAccess.Container),
			"envs":      paEnvs,
			"services": map[string]any{
				"https": map[string]any{
					"containerPort": paCfg.AdminPort,
					"servicePort":   paCfg.AdminPort,
					"dataService":   true,
					"ingressPort":   443,
				},
			},
			"ingress": paAdminIngressValues,
		}
		if len(paEnvFrom) > 0 {
			paAdminValues["envFrom"] = paEnvFrom
		}

		paEngineValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": spec.PingAccess.Replicas,
				},
			},
			"image":     paImageValues,
			"container": buildContainerValues(paCPU, paMem, spec.PingAccess.Container),
			"envs": paEnvs,
			"services": map[string]any{
				"https": map[string]any{
					"containerPort": paCfg.EnginePort,
					"servicePort":   paCfg.EnginePort,
					"dataService":   true,
					"ingressPort":   443,
				},
			},
			"ingress": paEngineIngressValues,
		}
		if len(paEnvFrom) > 0 {
			paEngineValues["envFrom"] = paEnvFrom
		}
	}

	// Assemble pingauthorize section
	var pazValues map[string]any
	if spec.PingAuthorize == nil {
		pazValues = map[string]any{"enabled": false}
	} else {
		pazCPU, pazMem := TierResources(spec.Tier, "pd")
		pazCfg := spec.PingAuthorize.Config
		pazSpec := spec.PingAuthorize
		pazIng := resolveIngressSpec(spec.Ingress, pazSpec.Ingress)
		pazIng.TLSSecretRef = resolveTLSSecretRef(pazIng.TLSSecretRef, spec.TenantID, "paz")
		pazHostname := resolveHostname(pazIng.Hostname, "paz", spec.Domain)

		pazEnvs := map[string]any{
			"USER_BASE_DN":          pazCfg.UserBaseDN,
			"LDAP_PORT":             fmt.Sprintf("%d", pazCfg.LDAPPort),
			"LDAPS_PORT":            fmt.Sprintf("%d", pazCfg.LDAPSPort),
			"HTTPS_PORT":            fmt.Sprintf("%d", pazCfg.HTTPSPort),
			"ADMIN_USER_NAME":       pazCfg.AdminUserName,
			"RETRY_TIMEOUT_SECONDS": fmt.Sprintf("%d", pazCfg.RetryTimeoutSeconds),
			"MAX_HEAP_SIZE":         pazCfg.MaxHeapSize,
			"TAIL_LOG_FILES":        "${SERVER_ROOT_DIR}/logs/access ${SERVER_ROOT_DIR}/logs/errors",
		}
		emitServerProfileEnvs(pazEnvs, pazCfg.ServerProfile, pazCfg.ServerProfileLayers)

		pazEnvFrom := map[string]any{}
		if pazCfg.AdminSecretRef != "" || pazCfg.EncryptionSecretRef != "" {
			secretRefs := []map[string]any{}
			if pazCfg.AdminSecretRef != "" {
				secretRefs = append(secretRefs, map[string]any{"name": pazCfg.AdminSecretRef})
			}
			if pazCfg.EncryptionSecretRef != "" {
				secretRefs = append(secretRefs, map[string]any{"name": pazCfg.EncryptionSecretRef})
			}
			pazEnvFrom["secretRef"] = secretRefs
		}
		if pazCfg.EnvConfigMapRef != "" {
			pazEnvFrom["configMapRef"] = []map[string]any{{"name": pazCfg.EnvConfigMapRef}}
		}

		var pazIngressValues map[string]any
		if ingressEnabled(pazIng) {
			pazIngressValues = buildIngressValues(pazIng, pazHostname)
		} else {
			pazIngressValues = map[string]any{"enabled": false}
		}

		pvcClaim := map[string]any{
			"accessModes": []string{"ReadWriteOnce"},
			"resources": map[string]any{
				"requests": map[string]any{"storage": pazSpec.StorageSize},
			},
		}
		if pazSpec.StorageClass != "" {
			pvcClaim["storageClassName"] = pazSpec.StorageClass
		}

		pazValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "StatefulSet",
				"statefulSet": map[string]any{
					"replicas":            pazSpec.Replicas,
					"podManagementPolicy": "OrderedReady",
					"persistentvolume": map[string]any{
						"enabled": true,
						"volumes": map[string]any{
							"out-dir": map[string]any{
								"mountPath":             "/opt/out",
								"persistentVolumeClaim": pvcClaim,
							},
						},
					},
				},
			},
			"image":     buildImageValues(pazSpec.Image, pazSpec.Version),
			"container": buildContainerValues(pazCPU, pazMem, pazSpec.Container),
			"envs":      pazEnvs,
			"services": map[string]any{
				"ldap": map[string]any{
					"containerPort":  pazCfg.LDAPPort,
					"servicePort":    pazCfg.LDAPPort,
					"clusterService": true,
				},
				"ldaps": map[string]any{
					"containerPort":  pazCfg.LDAPSPort,
					"servicePort":    pazCfg.LDAPSPort,
					"clusterService": true,
				},
				"https": map[string]any{
					"containerPort": pazCfg.HTTPSPort,
					"servicePort":   pazCfg.HTTPSPort,
					"dataService":   true,
				},
			},
			"ingress": pazIngressValues,
		}
		if len(pazEnvFrom) > 0 {
			pazValues["envFrom"] = pazEnvFrom
		}
	}

	// Assemble pingauthorizepap section
	var papValues map[string]any
	if spec.PingAuthorizePAP == nil {
		papValues = map[string]any{"enabled": false}
	} else {
		papCfg := spec.PingAuthorizePAP.Config
		papIng := resolveIngressSpec(spec.Ingress, spec.PingAuthorizePAP.Ingress)
		papIng.TLSSecretRef = resolveTLSSecretRef(papIng.TLSSecretRef, spec.TenantID, "paz-pap")
		papHostname := resolveHostname(papIng.Hostname, "paz-pap", spec.Domain)

		// Derive PING_EXTERNAL_BASE_URL from ingress hostname when not set
		externalBaseURL := papCfg.ExternalBaseURL
		if externalBaseURL == "" && papHostname != "" {
			externalBaseURL = "https://" + papHostname
		}

		papEnvs := map[string]any{
			"MAX_HEAP_SIZE":              papCfg.MaxHeapSize,
			"PING_ENABLE_API_HTTP_CACHE": fmt.Sprintf("%t", *papCfg.EnableAPIHTTPCache),
		}
		if externalBaseURL != "" {
			papEnvs["PING_EXTERNAL_BASE_URL"] = externalBaseURL
		}
		if papCfg.OIDCConfigEndpoint != "" {
			papEnvs["PING_OIDC_CONFIGURATION_ENDPOINT"] = papCfg.OIDCConfigEndpoint
		}
		if papCfg.ClientID != "" {
			papEnvs["PING_CLIENT_ID"] = papCfg.ClientID
		}
		if papCfg.PolicyDBSync {
			papEnvs["PING_POLICY_DB_SYNC"] = "true"
		}
		emitServerProfileEnvs(papEnvs, papCfg.ServerProfile, papCfg.ServerProfileLayers)

		papEnvFrom := map[string]any{}
		if papCfg.SharedSecretRef != "" {
			papEnvFrom["secretRef"] = []map[string]any{{"name": papCfg.SharedSecretRef}}
		}
		if papCfg.EnvConfigMapRef != "" {
			papEnvFrom["configMapRef"] = []map[string]any{{"name": papCfg.EnvConfigMapRef}}
		}

		var papIngressValues map[string]any
		if ingressEnabled(papIng) {
			papIngressValues = buildIngressValues(papIng, papHostname)
		} else {
			papIngressValues = map[string]any{"enabled": false}
		}

		papValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": 1,
				},
			},
			"image":   buildImageValues(spec.PingAuthorizePAP.Image, spec.PingAuthorizePAP.Version),
			"envs":    papEnvs,
			"ingress": papIngressValues,
		}
		if c := buildContainerValues("", "", spec.PingAuthorizePAP.Container); len(c) > 0 {
			papValues["container"] = c
		}
		if len(papEnvFrom) > 0 {
			papValues["envFrom"] = papEnvFrom
		}
	}

	values := map[string]any{
		"global":              globalValues,
		"pingfederate-admin":  pfAdminValues,
		"pingfederate-engine": pfEngineValues,
		"pingdirectory":       pdValues,
		"pingdataconsole":     pdcValues,
		"pingaccess-admin":    paAdminValues,
		"pingaccess-engine":   paEngineValues,
		"pingauthorize":       pazValues,
		"pingauthorizepap":    papValues,
	}

	// Merge PingFederate ValuesOverride
	var err error
	values, err = MergeValues(values, spec.PingFederate.ValuesOverride)
	if err != nil {
		return nil, fmt.Errorf("merge PingFederate valuesOverride: %w", err)
	}

	// Merge PingDirectory ValuesOverride
	if spec.PingDirectory != nil {
		values, err = MergeValues(values, spec.PingDirectory.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingDirectory valuesOverride: %w", err)
		}
	}

	// Merge PingAccess ValuesOverride
	if spec.PingAccess != nil {
		values, err = MergeValues(values, spec.PingAccess.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAccess valuesOverride: %w", err)
		}
	}

	// Merge PingAuthorize ValuesOverride
	if spec.PingAuthorize != nil {
		values, err = MergeValues(values, spec.PingAuthorize.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAuthorize valuesOverride: %w", err)
		}
	}

	// Merge PingAuthorizePAP ValuesOverride
	if spec.PingAuthorizePAP != nil {
		values, err = MergeValues(values, spec.PingAuthorizePAP.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAuthorizePAP valuesOverride: %w", err)
		}
	}

	return values, nil
}

// buildContainerValues constructs the container map, merging resource requests with
// any waitFor dependencies. Returns nil when there is nothing to set.
func buildContainerValues(cpu, memory string, container pingonev1alpha1.ContainerSpec) map[string]any {
	m := map[string]any{}
	if cpu != "" || memory != "" {
		req := map[string]any{}
		if cpu != "" {
			req["cpu"] = cpu
		}
		if memory != "" {
			req["memory"] = memory
		}
		m["resources"] = map[string]any{"requests": req}
	}
	if len(container.WaitFor) > 0 {
		wf := make(map[string]any, len(container.WaitFor))
		for svc, spec := range container.WaitFor {
			entry := map[string]any{"service": spec.Service}
			if spec.TimeoutSeconds > 0 {
				entry["timeoutSeconds"] = spec.TimeoutSeconds
			}
			wf[svc] = entry
		}
		m["waitFor"] = wf
	}
	return m
}

// emitServerProfileEnvs writes SERVER_PROFILE_* env vars from a layered profile spec.
// The base profile maps to SERVER_PROFILE_URL/_BRANCH/_PATH/_PARENT.
// Each entry in layers maps to SERVER_PROFILE_<UPPERCASED_KEY>_URL etc.
func emitServerProfileEnvs(envs map[string]any, profile *pingonev1alpha1.ServerProfileSpec, layers map[string]pingonev1alpha1.ServerProfileSpec) {
	if profile == nil {
		return
	}
	if profile.URL != "" {
		envs["SERVER_PROFILE_URL"] = profile.URL
	}
	if profile.Branch != "" {
		envs["SERVER_PROFILE_BRANCH"] = profile.Branch
	}
	if profile.Path != "" {
		envs["SERVER_PROFILE_PATH"] = profile.Path
	}
	if profile.Parent != "" {
		envs["SERVER_PROFILE_PARENT"] = profile.Parent
	}
	for name, layer := range layers {
		k := strings.ToUpper(name)
		if layer.URL != "" {
			envs["SERVER_PROFILE_"+k+"_URL"] = layer.URL
		}
		if layer.Branch != "" {
			envs["SERVER_PROFILE_"+k+"_BRANCH"] = layer.Branch
		}
		if layer.Path != "" {
			envs["SERVER_PROFILE_"+k+"_PATH"] = layer.Path
		}
		if layer.Parent != "" {
			envs["SERVER_PROFILE_"+k+"_PARENT"] = layer.Parent
		}
	}
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// buildImageValues returns the image map for a Helm sub-chart.
// The ping-devops chart constructs the final image as {repository}/{name}:{tag}.
//
// version may be a bare tag ("13.0.2-edge") or a full reference
// ("docker.io/pingidentity/pingfederate:13.0.2-edge"). A full reference (contains "/")
// is split into repository, name, and tag. The explicit repository argument overrides
// the repository parsed from version when set. Omitting everything lets the chart use
// its own defaults.
func buildImageValues(repository, version string) map[string]any {
	repo, name, tag := repository, "", version
	if strings.Contains(version, "/") {
		rest := version
		if idx := strings.LastIndex(rest, ":"); idx != -1 {
			tag = rest[idx+1:]
			rest = rest[:idx]
		} else {
			tag = ""
		}
		if idx := strings.LastIndex(rest, "/"); idx != -1 {
			if repository == "" {
				repo = rest[:idx]
			}
			name = rest[idx+1:]
		} else if repository == "" {
			repo = rest
		}
	}
	m := map[string]any{}
	if repo != "" {
		m["repository"] = repo
	}
	if name != "" {
		m["name"] = name
	}
	if tag != "" {
		m["tag"] = tag
	}
	return m
}

// resolveTLSSecretRef returns explicit if non-empty, otherwise "<tenantID>-<suffix>-tls".
func resolveTLSSecretRef(explicit, tenantID, suffix string) string {
	if explicit != "" {
		return explicit
	}
	return fmt.Sprintf("%s-%s-tls", tenantID, suffix)
}

// ingressEnabled returns true if the resolved ingress should be created.
func ingressEnabled(ing pingonev1alpha1.IngressSpec) bool {
	return ing.Enabled != nil && *ing.Enabled
}

// resolveIngressSpec merges global ingress defaults into a per-component IngressSpec.
// Component fields take precedence; annotations are merged with global as the base.
// enabled is inherited from global when the component does not set it explicitly.
func resolveIngressSpec(global pingonev1alpha1.GlobalIngressSpec, component pingonev1alpha1.IngressSpec) pingonev1alpha1.IngressSpec {
	if component.Enabled == nil {
		enabled := global.Enabled
		component.Enabled = &enabled
	}
	if component.ClassName == "" {
		component.ClassName = global.ClassName
	}
	if component.TLSSecretRef == "" {
		component.TLSSecretRef = global.TLSSecretRef
	}
	if len(global.Annotations) > 0 {
		merged := make(map[string]string, len(global.Annotations)+len(component.Annotations))
		for k, v := range global.Annotations {
			merged[k] = v
		}
		for k, v := range component.Annotations {
			merged[k] = v
		}
		component.Annotations = merged
	}
	return component
}

// resolveHostname returns explicit if non-empty, otherwise "<prefix>.<domain>" if domain is set, otherwise "".
func resolveHostname(explicit, prefix, domain string) string {
	if explicit != "" {
		return explicit
	}
	if domain != "" {
		return prefix + "." + domain
	}
	return ""
}

// buildIngressValues constructs the ingress values map for a product from an IngressSpec.
// hostname is the resolved FQDN (may differ from ing.Hostname when derived from spec.domain).
func buildIngressValues(ing pingonev1alpha1.IngressSpec, hostname string) map[string]any {
	m := map[string]any{
		"enabled": true,
		"hosts": []map[string]any{
			{
				"host": hostname,
				"paths": []map[string]any{
					{
						"path":     "/",
						"pathType": "Prefix",
						"backend": map[string]any{
							"serviceName": "https",
						},
					},
				},
			},
		},
	}
	if ing.ClassName != "" {
		m["spec"] = map[string]any{
			"ingressClassName": ing.ClassName,
		}
	}
	if ing.TLSSecretRef != "" && hostname != "" {
		m["tls"] = []map[string]any{
			{
				"secretName": ing.TLSSecretRef,
				"hosts":      []string{hostname},
			},
		}
	}
	if len(ing.Annotations) > 0 {
		m["annotations"] = ing.Annotations
	}
	return m
}

// mergeMaps recursively merges src into dst, returning the result.
// Values in src take precedence; nested maps are merged rather than replaced.
func mergeMaps(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if dv, ok := out[k]; ok {
			if dm, ok := dv.(map[string]any); ok {
				if sm, ok := v.(map[string]any); ok {
					out[k] = mergeMaps(dm, sm)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
