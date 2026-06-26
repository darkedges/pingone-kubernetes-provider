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

// ProductSpecs bundles the optional per-product specs contributed by each product CR.
// A nil entry means that product is not deployed.
type ProductSpecs struct {
	PingFederate       *pingonev1alpha1.PingFederateSpec
	PingDirectory      *pingonev1alpha1.PingDirectorySpec
	PingAccess         *pingonev1alpha1.PingAccessSpec
	PingAuthorize      *pingonev1alpha1.PingAuthorizeSpec
	PingAuthorizePAP   *pingonev1alpha1.PingAuthorizePAPSpec
	PingDataSync       *pingonev1alpha1.PingDataSyncSpec
	PingDirectoryProxy *pingonev1alpha1.PingDirectoryProxySpec
}

// BuildPingValues constructs the full Helm values map for a ping-devops release
// from a PingEnvironment spec and per-product specs. It applies defaults, builds
// the global section, per-product sub-chart sections, and merges any ValuesOverride on top.
func BuildPingValues(env pingonev1alpha1.PingEnvironmentSpec, products ProductSpecs) (map[string]any, error) {
	applyDefaults(&env, &products)

	// Build global section
	globalValues := map[string]any{
		"envs": map[string]any{
			"PING_IDENTITY_ACCEPT_EULA": "YES",
		},
		"ingress": map[string]any{
			"enabled": false,
		},
	}

	// Vault — global.vault
	if v := rawToMap(env.Vault); v != nil {
		globalValues["vault"] = v
	}

	// Global workload securityContext and container securityContext
	// → global.workload.securityContext / global.workload.container.securityContext
	globalWorkload := map[string]any{}
	if sc := rawToMap(env.SecurityContext); sc != nil {
		globalWorkload["securityContext"] = sc
	}
	if csc := rawToMap(env.ContainerSecurityContext); csc != nil {
		globalWorkload["container"] = map[string]any{"securityContext": csc}
	}
	if len(globalWorkload) > 0 {
		globalValues["workload"] = globalWorkload
	}

	// Global resources — global.container.resources
	if env.Resources != nil {
		globalValues["container"] = map[string]any{
			"resources": buildResourcesMap(env.Resources),
		}
	}

	// Build PingFederate sections
	var pfAdminValues, pfEngineValues map[string]any
	if products.PingFederate == nil {
		pfAdminValues = map[string]any{"enabled": false}
		pfEngineValues = map[string]any{"enabled": false}
	} else {
		pfCPU, pfMem := TierResources(env.Tier, "pf")
		pfCfg := products.PingFederate.Config
		pfIng := resolveIngressSpec(env.Ingress, products.PingFederate.EngineIngress)
		pfAdminIng := resolveIngressSpec(env.Ingress, products.PingFederate.AdminIngress)

		// Resolve hostnames from domain when not explicitly set
		engineHostname := resolveHostname(pfIng.Hostname, "pf", env.Domain)
		adminHostname := resolveHostname(pfAdminIng.Hostname, "pf-admin", env.Domain)

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

		// Operational mode — only set when explicitly configured; chart defaults to STANDALONE
		if pfCfg.OperationalMode != "" {
			pfEnvs["OPERATIONAL_MODE"] = pfCfg.OperationalMode
			pfEnvs["CLUSTER_BIND_ADDRESS"] = "NON_LOOPBACK"
		}

		// Authentication — only set when explicitly configured
		if pfCfg.ConsoleAuthentication != "" {
			pfEnvs["PF_CONSOLE_AUTHENTICATION"] = pfCfg.ConsoleAuthentication
		}
		if pfCfg.AdminAPIAuthentication != "" {
			pfEnvs["PF_ADMIN_API_AUTHENTICATION"] = pfCfg.AdminAPIAuthentication
		}
		if pfCfg.LDAPType != "" {
			pfEnvs["PF_LDAP_TYPE"] = pfCfg.LDAPType
		}
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

		// Provisioner — only set when explicitly configured
		if pfCfg.ProvisionerMode != "" {
			pfEnvs["PF_PROVISIONER_MODE"] = pfCfg.ProvisionerMode
		}
		if pfCfg.ProvisionerNodeID != 0 {
			pfEnvs["PF_PROVISIONER_NODE_ID"] = fmt.Sprintf("%d", pfCfg.ProvisionerNodeID)
		}
		if pfCfg.ProvisionerGracePeriod != 0 {
			pfEnvs["PF_PROVISIONER_GRACE_PERIOD"] = fmt.Sprintf("%d", pfCfg.ProvisionerGracePeriod)
		} else if pfCfg.ProvisionerNodeID != 0 {
			pfEnvs["PF_PROVISIONER_GRACE_PERIOD"] = "600"
		}

		// JVM
		pfEnvs["JAVA_RAM_PERCENTAGE"] = pfCfg.JavaRAMPercentage

		// HSM — only set when explicitly configured
		if pfCfg.HSMMode != "" {
			pfEnvs["HSM_MODE"] = pfCfg.HSMMode
		}
		if pfCfg.HSMHybrid {
			pfEnvs["PF_HSM_HYBRID"] = "true"
		}
		if pfCfg.BCFIPSApprovedOnly {
			pfEnvs["PF_BC_FIPS_APPROVED_ONLY"] = "true"
		}
		if pfCfg.EngineDebug {
			pfEnvs["PF_ENGINE_DEBUG"] = "true"
		}
		if pfCfg.AdminDebug {
			pfEnvs["PF_ADMIN_DEBUG"] = "true"
		}
		if pfCfg.DebugPort != 0 {
			pfEnvs["PF_DEBUG_PORT"] = fmt.Sprintf("%d", pfCfg.DebugPort)
		}
		if pfCfg.EngineSecondaryPort != 0 {
			pfEnvs["PF_ENGINE_SECONDARY_PORT"] = fmt.Sprintf("%d", pfCfg.EngineSecondaryPort)
		}
		if pfCfg.NodeTags != "" {
			pfEnvs["PF_NODE_TAGS"] = pfCfg.NodeTags
		}
		if pfCfg.AdminWaitForTimeout != 0 {
			pfEnvs["ADMIN_WAITFOR_TIMEOUT"] = fmt.Sprintf("%d", pfCfg.AdminWaitForTimeout)
		}
		if pfCfg.LogSizeMax != "" {
			pfEnvs["PF_LOG_SIZE_MAX"] = pfCfg.LogSizeMax
		}
		if pfCfg.LogNumber != 0 {
			pfEnvs["PF_LOG_NUMBER"] = fmt.Sprintf("%d", pfCfg.LogNumber)
		}
		if pfCfg.JettyThreadsMin != 0 {
			pfEnvs["PF_JETTY_THREADS_MIN"] = fmt.Sprintf("%d", pfCfg.JettyThreadsMin)
		}
		if pfCfg.JettyThreadsMax != 0 {
			pfEnvs["PF_JETTY_THREADS_MAX"] = fmt.Sprintf("%d", pfCfg.JettyThreadsMax)
		}
		if pfCfg.AcceptQueueSize != 0 {
			pfEnvs["PF_ACCEPT_QUEUE_SIZE"] = fmt.Sprintf("%d", pfCfg.AcceptQueueSize)
		}
		if pfCfg.CreateInitialAdminUser {
			pfEnvs["CREATE_INITIAL_ADMIN_USER"] = "true"
		}
		if pfCfg.EnableAutomaticHeapDump != nil {
			pfEnvs["ENABLE_AUTOMATIC_HEAP_DUMP"] = fmt.Sprintf("%t", *pfCfg.EnableAutomaticHeapDump)
		}

		// Logging
		pfEnvs["TAIL_LOG_FILES"] = "${SERVER_ROOT_DIR}/log/server.log"

		for k, v := range pfCfg.Envs {
			pfEnvs[k] = v
		}

		// Build PingFederate envFrom (container.envFrom list)
		var pfEnvFrom []map[string]any
		if pfCfg.AdminSecretRef != "" {
			pfEnvFrom = append(pfEnvFrom, secretEnvFrom(pfCfg.AdminSecretRef))
		}
		if pfCfg.LDAPSecretRef != "" {
			pfEnvFrom = append(pfEnvFrom, secretEnvFrom(pfCfg.LDAPSecretRef))
		}
		if pfCfg.EnvConfigMapRef != "" {
			pfEnvFrom = append(pfEnvFrom, configMapEnvFrom(pfCfg.EnvConfigMapRef))
		}
		pfContainerVals := buildContainerValues(pfCPU, pfMem, products.PingFederate.Container)
		if len(pfEnvFrom) > 0 {
			pfContainerVals["envFrom"] = pfEnvFrom
		}

		// Auto-derive TLS secret names when not explicitly set
		pfIng.TLSSecretRef = resolveTLSSecretRef(pfIng.TLSSecretRef, env.TenantID, "pf")
		pfAdminIng.TLSSecretRef = resolveTLSSecretRef(pfAdminIng.TLSSecretRef, env.TenantID, "pf-admin")

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

		pfImageValues := buildImageValues(products.PingFederate.Image, products.PingFederate.Version)

		// Assemble pingfederate-admin section (admin console, 1 replica)
		pfAdminValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": 1,
				},
			},
			"image":     pfImageValues,
			"container": pfContainerVals,
			"envs":      pfEnvs,
			"services": map[string]any{
				"https": adminSvc,
			},
			"ingress": pfAdminIngressValues,
		}

		// Assemble pingfederate-engine section (runtime engine, user-specified replicas)
		pfEngineValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": products.PingFederate.Replicas,
				},
			},
			"image":     pfImageValues,
			"container": pfContainerVals,
			"envs":      pfEnvs,
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
		applyWorkloadSecurityContext(pfAdminValues, products.PingFederate.Container.SecurityContext)
		applyWorkloadSecurityContext(pfEngineValues, products.PingFederate.Container.SecurityContext)
		applyRawVolumes(pfAdminValues, products.PingFederate.Container)
		applyRawVolumes(pfEngineValues, products.PingFederate.Container)
		pfSvcAnnotations := resolveServiceAnnotations(env.Services, products.PingFederate.Service)
		applyServiceAnnotations(pfAdminValues, pfSvcAnnotations)
		applyServiceAnnotations(pfEngineValues, pfSvcAnnotations)
	} // end PingFederate

	// Assemble pingdirectory section
	var pdValues map[string]any
	if products.PingDirectory == nil {
		pdValues = map[string]any{"enabled": false}
	} else {
		pdCPU, pdMem := TierResources(env.Tier, "pd")
		pdCfg := products.PingDirectory.Config
		pdSpec := products.PingDirectory

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
		pdEnvs["FAIL_ON_UNSUCCESSFUL_REMOVE_DEFUNCT"] = fmt.Sprintf("%t", pdCfg.FailOnUnsuccessfulRemoveDefunct)
		pdEnvs["PD_FORCE_DATA_REIMPORT"] = fmt.Sprintf("%t", pdCfg.ForceDataReimport)
		pdEnvs["SKIP_WAIT_FOR_DNS"] = fmt.Sprintf("%t", pdCfg.SkipWaitForDNS)
		pdEnvs["PARALLEL_POD_MANAGEMENT_POLICY"] = fmt.Sprintf("%t", pdCfg.ParallelPodManagement)
		if pdCfg.LoadBalancingAlgorithmNames != "" {
			pdEnvs["LOAD_BALANCING_ALGORITHM_NAMES"] = pdCfg.LoadBalancingAlgorithmNames
		}
		if pdCfg.RestrictedBaseDNs != "" {
			pdEnvs["RESTRICTED_BASE_DNS"] = pdCfg.RestrictedBaseDNs
		}
		if pdCfg.CertificateNickname != "" {
			pdEnvs["CERTIFICATE_NICKNAME"] = pdCfg.CertificateNickname
		}
		if pdCfg.KeystoreFile != "" {
			pdEnvs["KEYSTORE_FILE"] = pdCfg.KeystoreFile
		}
		if pdCfg.KeystorePinFile != "" {
			pdEnvs["KEYSTORE_PIN_FILE"] = pdCfg.KeystorePinFile
		}
		if pdCfg.KeystoreType != "" {
			pdEnvs["KEYSTORE_TYPE"] = pdCfg.KeystoreType
		}
		if pdCfg.TruststoreFile != "" {
			pdEnvs["TRUSTSTORE_FILE"] = pdCfg.TruststoreFile
		}
		if pdCfg.TruststorePinFile != "" {
			pdEnvs["TRUSTSTORE_PIN_FILE"] = pdCfg.TruststorePinFile
		}
		if pdCfg.TruststoreType != "" {
			pdEnvs["TRUSTSTORE_TYPE"] = pdCfg.TruststoreType
		}
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

		// PingDirectory envFrom (container.envFrom list)
		var pdEnvFrom []map[string]any
		if pdCfg.AdminSecretRef != "" {
			pdEnvFrom = append(pdEnvFrom, secretEnvFrom(pdCfg.AdminSecretRef))
		}
		if pdCfg.EncryptionSecretRef != "" {
			pdEnvFrom = append(pdEnvFrom, secretEnvFrom(pdCfg.EncryptionSecretRef))
		}
		if pdCfg.KeystoreSecretRef != "" {
			pdEnvFrom = append(pdEnvFrom, secretEnvFrom(pdCfg.KeystoreSecretRef))
		}
		if pdCfg.TruststoreSecretRef != "" {
			pdEnvFrom = append(pdEnvFrom, secretEnvFrom(pdCfg.TruststoreSecretRef))
		}
		for k, v := range pdCfg.Envs {
			pdEnvs[k] = v
		}
		if pdCfg.EnvConfigMapRef != "" {
			pdEnvFrom = append(pdEnvFrom, configMapEnvFrom(pdCfg.EnvConfigMapRef))
		}
		pdContainerVals := buildContainerValues(pdCPU, pdMem, pdSpec.Container)
		if len(pdEnvFrom) > 0 {
			pdContainerVals["envFrom"] = pdEnvFrom
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
			"container": pdContainerVals,
			"envs":      pdEnvs,
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
		applyWorkloadSecurityContext(pdValues, products.PingDirectory.Container.SecurityContext)
		applyRawVolumes(pdValues, products.PingDirectory.Container)
		applyServiceAnnotations(pdValues, resolveServiceAnnotations(env.Services, products.PingDirectory.Service))
	}

	// Assemble pingdataconsole section.
	// Only deployed when spec.pingDataConsole is explicitly set and not disabled.
	var pdcValues map[string]any
	pdcShouldEnable := products.PingDirectory != nil &&
		env.PingDataConsole != nil &&
		(env.PingDataConsole.Enabled == nil || *env.PingDataConsole.Enabled)
	if pdcShouldEnable {
		var pdc pingonev1alpha1.PingDataConsoleSpec
		if env.PingDataConsole != nil {
			pdc = *env.PingDataConsole
		}
		pdcIng := resolveIngressSpec(env.Ingress, pdc.Ingress)
		pdcIng.TLSSecretRef = resolveTLSSecretRef(pdcIng.TLSSecretRef, env.TenantID, "pd-console")
		pdcHostname := resolveHostname(pdcIng.Hostname, "pd-console", env.Domain)

		var pdcIngressValues map[string]any
		if ingressEnabled(pdcIng) {
			pdcIngressValues = buildIngressValues(pdcIng, pdcHostname)
		} else {
			pdcIngressValues = map[string]any{"enabled": false}
		}

		// Cluster service name for PingDirectory: <tenantId>-ping-pingdirectory-cluster
		pdClusterSvc := fmt.Sprintf("%s-ping-pingdirectory-cluster", env.TenantID)

		// Console image: use explicit image/version, fall back to PingDirectory's values
		pdcImage := buildImageValues(
			firstNonEmpty(pdc.Image, products.PingDirectory.Image),
			firstNonEmpty(pdc.Version, products.PingDirectory.Version),
		)

		pdcEnvs := map[string]any{}
		if pdc.HTTPPort != 0 {
			pdcEnvs["HTTP_PORT"] = fmt.Sprintf("%d", pdc.HTTPPort)
		}
		if pdc.HTTPSPort != 0 {
			pdcEnvs["HTTPS_PORT"] = fmt.Sprintf("%d", pdc.HTTPSPort)
		}
		if pdc.BrandingAppName != "" {
			pdcEnvs["BRANDING_APP_NAME"] = pdc.BrandingAppName
		}
		if pdc.SystemReadOnly {
			pdcEnvs["SYSTEM_READ_ONLY"] = "true"
		}

		pdcValues = map[string]any{
			"enabled": true,
			"image":   pdcImage,
			"defaultLogin": map[string]any{
				"server": map[string]any{
					"host": pdClusterSvc,
					"port": products.PingDirectory.Config.LDAPSPort,
				},
			},
			"ingress": pdcIngressValues,
		}
		if len(pdcEnvs) > 0 {
			pdcValues["envs"] = pdcEnvs
		}
	} else {
		pdcValues = map[string]any{"enabled": false}
	}

	// Assemble pingaccess-admin and pingaccess-engine sections
	var paAdminValues, paEngineValues map[string]any
	if products.PingAccess == nil {
		paAdminValues = map[string]any{"enabled": false}
		paEngineValues = map[string]any{"enabled": false}
	} else {
		paCPU, paMem := TierResources(env.Tier, "pf")
		paCfg := products.PingAccess.Config
		paAdminIng := resolveIngressSpec(env.Ingress, products.PingAccess.AdminIngress)
		paEngineIng := resolveIngressSpec(env.Ingress, products.PingAccess.EngineIngress)
		paAdminIng.TLSSecretRef = resolveTLSSecretRef(paAdminIng.TLSSecretRef, env.TenantID, "pa-admin")
		paEngineIng.TLSSecretRef = resolveTLSSecretRef(paEngineIng.TLSSecretRef, env.TenantID, "pa")
		paAdminHostname := resolveHostname(paAdminIng.Hostname, "pa-admin", env.Domain)
		paEngineHostname := resolveHostname(paEngineIng.Hostname, "pa", env.Domain)

		paEnvs := map[string]any{
			"PA_ADMIN_PORT":       fmt.Sprintf("%d", paCfg.AdminPort),
			"PA_ENGINE_PORT":      fmt.Sprintf("%d", paCfg.EnginePort),
			"JAVA_RAM_PERCENTAGE": paCfg.JavaRAMPercentage,
			"TAIL_LOG_FILES":      "${SERVER_ROOT_DIR}/log/pingaccess.log",
		}
		if paCfg.OperationalMode != "" {
			paEnvs["OPERATIONAL_MODE"] = paCfg.OperationalMode
		}
		if paCfg.FIPSModeOn {
			paEnvs["FIPS_MODE_ON"] = "true"
		}
		emitServerProfileEnvs(paEnvs, paCfg.ServerProfile, paCfg.ServerProfileLayers)
		if paCfg.AdminPublicHostname != "" {
			paEnvs["PA_ADMIN_PUBLIC_HOSTNAME"] = paCfg.AdminPublicHostname
		}
		if paCfg.EnginePublicHostname != "" {
			paEnvs["PA_ENGINE_PUBLIC_HOSTNAME"] = paCfg.EnginePublicHostname
		}
		if paCfg.AdminWaitForTimeout != 0 {
			paEnvs["ADMIN_WAITFOR_TIMEOUT"] = fmt.Sprintf("%d", paCfg.AdminWaitForTimeout)
		}

		for k, v := range paCfg.Envs {
			paEnvs[k] = v
		}
		var paEnvFrom []map[string]any
		if paCfg.AdminSecretRef != "" {
			paEnvFrom = append(paEnvFrom, secretEnvFrom(paCfg.AdminSecretRef))
		}
		if paCfg.EnvConfigMapRef != "" {
			paEnvFrom = append(paEnvFrom, configMapEnvFrom(paCfg.EnvConfigMapRef))
		}
		paContainerVals := buildContainerValues(paCPU, paMem, products.PingAccess.Container)
		if len(paEnvFrom) > 0 {
			paContainerVals["envFrom"] = paEnvFrom
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

		paImageValues := buildImageValues(products.PingAccess.Image, products.PingAccess.Version)

		paAdminValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": 1,
				},
			},
			"image":     paImageValues,
			"container": paContainerVals,
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

		paEngineValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": products.PingAccess.Replicas,
				},
			},
			"image":     paImageValues,
			"container": paContainerVals,
			"envs":      paEnvs,
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
		applyWorkloadSecurityContext(paAdminValues, products.PingAccess.Container.SecurityContext)
		applyWorkloadSecurityContext(paEngineValues, products.PingAccess.Container.SecurityContext)
		applyRawVolumes(paAdminValues, products.PingAccess.Container)
		applyRawVolumes(paEngineValues, products.PingAccess.Container)
		paSvcAnnotations := resolveServiceAnnotations(env.Services, products.PingAccess.Service)
		applyServiceAnnotations(paAdminValues, paSvcAnnotations)
		applyServiceAnnotations(paEngineValues, paSvcAnnotations)
	}

	// Assemble pingauthorize section
	var pazValues map[string]any
	if products.PingAuthorize == nil {
		pazValues = map[string]any{"enabled": false}
	} else {
		pazCPU, pazMem := TierResources(env.Tier, "pd")
		pazCfg := products.PingAuthorize.Config
		pazSpec := products.PingAuthorize
		pazIng := resolveIngressSpec(env.Ingress, pazSpec.Ingress)
		pazIng.TLSSecretRef = resolveTLSSecretRef(pazIng.TLSSecretRef, env.TenantID, "paz")
		pazHostname := resolveHostname(pazIng.Hostname, "paz", env.Domain)

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

		for k, v := range pazCfg.Envs {
			pazEnvs[k] = v
		}
		var pazEnvFrom []map[string]any
		if pazCfg.AdminSecretRef != "" {
			pazEnvFrom = append(pazEnvFrom, secretEnvFrom(pazCfg.AdminSecretRef))
		}
		if pazCfg.EncryptionSecretRef != "" {
			pazEnvFrom = append(pazEnvFrom, secretEnvFrom(pazCfg.EncryptionSecretRef))
		}
		if pazCfg.EnvConfigMapRef != "" {
			pazEnvFrom = append(pazEnvFrom, configMapEnvFrom(pazCfg.EnvConfigMapRef))
		}
		pazContainerVals := buildContainerValues(pazCPU, pazMem, pazSpec.Container)
		if len(pazEnvFrom) > 0 {
			pazContainerVals["envFrom"] = pazEnvFrom
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
			"container": pazContainerVals,
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
		applyWorkloadSecurityContext(pazValues, products.PingAuthorize.Container.SecurityContext)
		applyRawVolumes(pazValues, products.PingAuthorize.Container)
		applyServiceAnnotations(pazValues, resolveServiceAnnotations(env.Services, products.PingAuthorize.Service))
	}

	// Assemble pingauthorizepap section
	var papValues map[string]any
	if products.PingAuthorizePAP == nil {
		papValues = map[string]any{"enabled": false}
	} else {
		papCfg := products.PingAuthorizePAP.Config
		papIng := resolveIngressSpec(env.Ingress, products.PingAuthorizePAP.Ingress)
		papIng.TLSSecretRef = resolveTLSSecretRef(papIng.TLSSecretRef, env.TenantID, "paz-pap")
		papHostname := resolveHostname(papIng.Hostname, "paz-pap", env.Domain)

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
		if papCfg.DBConnectionString != "" {
			papEnvs["PING_DB_CONNECTION_STRING"] = papCfg.DBConnectionString
		}
		if papCfg.DBAdminUsername != "" {
			papEnvs["PING_DB_ADMIN_USERNAME"] = papCfg.DBAdminUsername
		}
		if papCfg.DBAppUsername != "" {
			papEnvs["PING_DB_APP_USERNAME"] = papCfg.DBAppUsername
		}
		if papCfg.KeystoreFile != "" {
			papEnvs["KEYSTORE_FILE"] = papCfg.KeystoreFile
		}
		if papCfg.KeystorePinFile != "" {
			papEnvs["KEYSTORE_PIN_FILE"] = papCfg.KeystorePinFile
		}
		if papCfg.KeystoreType != "" {
			papEnvs["KEYSTORE_TYPE"] = papCfg.KeystoreType
		}
		emitServerProfileEnvs(papEnvs, papCfg.ServerProfile, papCfg.ServerProfileLayers)

		var papEnvFrom []map[string]any
		if papCfg.SharedSecretRef != "" {
			papEnvFrom = append(papEnvFrom, secretEnvFrom(papCfg.SharedSecretRef))
		}
		if papCfg.DBSecretRef != "" {
			papEnvFrom = append(papEnvFrom, secretEnvFrom(papCfg.DBSecretRef))
		}
		if papCfg.KeystoreSecretRef != "" {
			papEnvFrom = append(papEnvFrom, secretEnvFrom(papCfg.KeystoreSecretRef))
		}
		for k, v := range papCfg.Envs {
			papEnvs[k] = v
		}
		if papCfg.EnvConfigMapRef != "" {
			papEnvFrom = append(papEnvFrom, configMapEnvFrom(papCfg.EnvConfigMapRef))
		}

		var papIngressValues map[string]any
		if ingressEnabled(papIng) {
			papIngressValues = buildIngressValues(papIng, papHostname)
		} else {
			papIngressValues = map[string]any{"enabled": false}
		}

		papContainerVals := buildContainerValues("", "", products.PingAuthorizePAP.Container)
		if len(papEnvFrom) > 0 {
			papContainerVals["envFrom"] = papEnvFrom
		}
		papValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "Deployment",
				"deployment": map[string]any{
					"replicas": 1,
				},
			},
			"image":     buildImageValues(products.PingAuthorizePAP.Image, products.PingAuthorizePAP.Version),
			"container": papContainerVals,
			"envs":      papEnvs,
			"ingress":   papIngressValues,
		}
		applyWorkloadSecurityContext(papValues, products.PingAuthorizePAP.Container.SecurityContext)
		applyRawVolumes(papValues, products.PingAuthorizePAP.Container)
		applyServiceAnnotations(papValues, resolveServiceAnnotations(env.Services, products.PingAuthorizePAP.Service))
	}

	// Assemble pingdatasync section
	var pdsValues map[string]any
	if products.PingDataSync == nil {
		pdsValues = map[string]any{"enabled": false}
	} else {
		pdsCPU, pdsMem := TierResources(env.Tier, "pd")
		pdsCfg := products.PingDataSync.Config
		pdsSpec := products.PingDataSync

		pdsEnvs := map[string]any{}
		emitServerProfileEnvs(pdsEnvs, pdsCfg.ServerProfile, pdsCfg.ServerProfileLayers)
		if pdsCfg.AdminUserName != "" {
			pdsEnvs["ADMIN_USER_NAME"] = pdsCfg.AdminUserName
		}
		if pdsCfg.RetryTimeoutSeconds != 0 {
			pdsEnvs["RETRY_TIMEOUT_SECONDS"] = fmt.Sprintf("%d", pdsCfg.RetryTimeoutSeconds)
		}
		if pdsCfg.RebuildOnRestart {
			pdsEnvs["PD_REBUILD_ON_RESTART"] = "true"
		}
		if pdsCfg.ParallelPodManagement {
			pdsEnvs["PARALLEL_POD_MANAGEMENT_POLICY"] = "true"
		}
		if pdsCfg.SkipWaitForDNS {
			pdsEnvs["SKIP_WAIT_FOR_DNS"] = "true"
		}
		if pdsCfg.CertificateNickname != "" {
			pdsEnvs["CERTIFICATE_NICKNAME"] = pdsCfg.CertificateNickname
		}
		if pdsCfg.KeystoreFile != "" {
			pdsEnvs["KEYSTORE_FILE"] = pdsCfg.KeystoreFile
		}
		if pdsCfg.KeystorePinFile != "" {
			pdsEnvs["KEYSTORE_PIN_FILE"] = pdsCfg.KeystorePinFile
		}
		if pdsCfg.KeystoreType != "" {
			pdsEnvs["KEYSTORE_TYPE"] = pdsCfg.KeystoreType
		}
		if pdsCfg.TruststoreFile != "" {
			pdsEnvs["TRUSTSTORE_FILE"] = pdsCfg.TruststoreFile
		}
		if pdsCfg.TruststorePinFile != "" {
			pdsEnvs["TRUSTSTORE_PIN_FILE"] = pdsCfg.TruststorePinFile
		}
		if pdsCfg.TruststoreType != "" {
			pdsEnvs["TRUSTSTORE_TYPE"] = pdsCfg.TruststoreType
		}

		pdsPodMgmtPolicy := "OrderedReady"
		if pdsCfg.ParallelPodManagement {
			pdsPodMgmtPolicy = "Parallel"
		}

		pdsPVCClaim := map[string]any{
			"accessModes": []string{"ReadWriteOnce"},
			"resources": map[string]any{
				"requests": map[string]any{"storage": pdsSpec.StorageSize},
			},
		}
		if pdsSpec.StorageClass != "" {
			pdsPVCClaim["storageClassName"] = pdsSpec.StorageClass
		}

		var pdsEnvFrom []map[string]any
		if pdsCfg.AdminSecretRef != "" {
			pdsEnvFrom = append(pdsEnvFrom, secretEnvFrom(pdsCfg.AdminSecretRef))
		}
		if pdsCfg.KeystoreSecretRef != "" {
			pdsEnvFrom = append(pdsEnvFrom, secretEnvFrom(pdsCfg.KeystoreSecretRef))
		}
		if pdsCfg.TruststoreSecretRef != "" {
			pdsEnvFrom = append(pdsEnvFrom, secretEnvFrom(pdsCfg.TruststoreSecretRef))
		}
		for k, v := range pdsCfg.Envs {
			pdsEnvs[k] = v
		}
		if pdsCfg.EnvConfigMapRef != "" {
			pdsEnvFrom = append(pdsEnvFrom, configMapEnvFrom(pdsCfg.EnvConfigMapRef))
		}

		pdsIng := resolveIngressSpec(env.Ingress, pdsSpec.Ingress)
		pdsIng.TLSSecretRef = resolveTLSSecretRef(pdsIng.TLSSecretRef, env.TenantID, "pds")
		pdsHostname := resolveHostname(pdsIng.Hostname, "pds", env.Domain)
		var pdsIngressValues map[string]any
		if ingressEnabled(pdsIng) {
			pdsIngressValues = buildIngressValues(pdsIng, pdsHostname)
		} else {
			pdsIngressValues = map[string]any{"enabled": false}
		}

		pdsContainerVals := buildContainerValues(pdsCPU, pdsMem, pdsSpec.Container)
		if len(pdsEnvFrom) > 0 {
			pdsContainerVals["envFrom"] = pdsEnvFrom
		}
		pdsValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "StatefulSet",
				"statefulSet": map[string]any{
					"replicas":            pdsSpec.Replicas,
					"podManagementPolicy": pdsPodMgmtPolicy,
					"persistentvolume": map[string]any{
						"enabled": true,
						"volumes": map[string]any{
							"out-dir": map[string]any{
								"mountPath":             "/opt/out",
								"persistentVolumeClaim": pdsPVCClaim,
							},
						},
					},
				},
			},
			"image":     buildImageValues(pdsSpec.Image, pdsSpec.Version),
			"container": pdsContainerVals,
			"envs":      pdsEnvs,
			"ingress":   pdsIngressValues,
		}
		applyWorkloadSecurityContext(pdsValues, products.PingDataSync.Container.SecurityContext)
		applyRawVolumes(pdsValues, products.PingDataSync.Container)
		applyServiceAnnotations(pdsValues, resolveServiceAnnotations(env.Services, products.PingDataSync.Service))
	}

	// Assemble pingdirectoryproxy section
	var pdpValues map[string]any
	if products.PingDirectoryProxy == nil {
		pdpValues = map[string]any{"enabled": false}
	} else {
		pdpCPU, pdpMem := TierResources(env.Tier, "pd")
		pdpCfg := products.PingDirectoryProxy.Config
		pdpSpec := products.PingDirectoryProxy

		pdpEnvs := map[string]any{}
		emitServerProfileEnvs(pdpEnvs, pdpCfg.ServerProfile, pdpCfg.ServerProfileLayers)
		if pdpCfg.AdminUserName != "" {
			pdpEnvs["ADMIN_USER_NAME"] = pdpCfg.AdminUserName
		}
		if pdpCfg.RetryTimeoutSeconds != 0 {
			pdpEnvs["RETRY_TIMEOUT_SECONDS"] = fmt.Sprintf("%d", pdpCfg.RetryTimeoutSeconds)
		}
		if pdpCfg.CertificateNickname != "" {
			pdpEnvs["CERTIFICATE_NICKNAME"] = pdpCfg.CertificateNickname
		}
		if pdpCfg.KeystoreFile != "" {
			pdpEnvs["KEYSTORE_FILE"] = pdpCfg.KeystoreFile
		}
		if pdpCfg.KeystorePinFile != "" {
			pdpEnvs["KEYSTORE_PIN_FILE"] = pdpCfg.KeystorePinFile
		}
		if pdpCfg.KeystoreType != "" {
			pdpEnvs["KEYSTORE_TYPE"] = pdpCfg.KeystoreType
		}
		if pdpCfg.TruststoreFile != "" {
			pdpEnvs["TRUSTSTORE_FILE"] = pdpCfg.TruststoreFile
		}
		if pdpCfg.TruststorePinFile != "" {
			pdpEnvs["TRUSTSTORE_PIN_FILE"] = pdpCfg.TruststorePinFile
		}
		if pdpCfg.TruststoreType != "" {
			pdpEnvs["TRUSTSTORE_TYPE"] = pdpCfg.TruststoreType
		}
		if pdpCfg.PingDirectoryHostname != "" {
			pdpEnvs["PINGDIRECTORY_HOSTNAME"] = pdpCfg.PingDirectoryHostname
		}
		if pdpCfg.PingDirectoryLDAPSPort != 0 {
			pdpEnvs["PINGDIRECTORY_LDAPS_PORT"] = fmt.Sprintf("%d", pdpCfg.PingDirectoryLDAPSPort)
		}
		if pdpCfg.JoinPDTopology {
			pdpEnvs["JOIN_PD_TOPOLOGY"] = "true"
		}

		pdpPVCClaim := map[string]any{
			"accessModes": []string{"ReadWriteOnce"},
			"resources": map[string]any{
				"requests": map[string]any{"storage": pdpSpec.StorageSize},
			},
		}
		if pdpSpec.StorageClass != "" {
			pdpPVCClaim["storageClassName"] = pdpSpec.StorageClass
		}

		var pdpEnvFrom []map[string]any
		if pdpCfg.AdminSecretRef != "" {
			pdpEnvFrom = append(pdpEnvFrom, secretEnvFrom(pdpCfg.AdminSecretRef))
		}
		if pdpCfg.KeystoreSecretRef != "" {
			pdpEnvFrom = append(pdpEnvFrom, secretEnvFrom(pdpCfg.KeystoreSecretRef))
		}
		if pdpCfg.TruststoreSecretRef != "" {
			pdpEnvFrom = append(pdpEnvFrom, secretEnvFrom(pdpCfg.TruststoreSecretRef))
		}
		for k, v := range pdpCfg.Envs {
			pdpEnvs[k] = v
		}
		if pdpCfg.EnvConfigMapRef != "" {
			pdpEnvFrom = append(pdpEnvFrom, configMapEnvFrom(pdpCfg.EnvConfigMapRef))
		}

		pdpIng := resolveIngressSpec(env.Ingress, pdpSpec.Ingress)
		pdpIng.TLSSecretRef = resolveTLSSecretRef(pdpIng.TLSSecretRef, env.TenantID, "pdp")
		pdpHostname := resolveHostname(pdpIng.Hostname, "pdp", env.Domain)
		var pdpIngressValues map[string]any
		if ingressEnabled(pdpIng) {
			pdpIngressValues = buildIngressValues(pdpIng, pdpHostname)
		} else {
			pdpIngressValues = map[string]any{"enabled": false}
		}

		pdpContainerVals := buildContainerValues(pdpCPU, pdpMem, pdpSpec.Container)
		if len(pdpEnvFrom) > 0 {
			pdpContainerVals["envFrom"] = pdpEnvFrom
		}
		pdpValues = map[string]any{
			"enabled": true,
			"workload": map[string]any{
				"type": "StatefulSet",
				"statefulSet": map[string]any{
					"replicas":            pdpSpec.Replicas,
					"podManagementPolicy": "OrderedReady",
					"persistentvolume": map[string]any{
						"enabled": true,
						"volumes": map[string]any{
							"out-dir": map[string]any{
								"mountPath":             "/opt/out",
								"persistentVolumeClaim": pdpPVCClaim,
							},
						},
					},
				},
			},
			"image":     buildImageValues(pdpSpec.Image, pdpSpec.Version),
			"container": pdpContainerVals,
			"envs":      pdpEnvs,
			"ingress":   pdpIngressValues,
		}
		applyWorkloadSecurityContext(pdpValues, products.PingDirectoryProxy.Container.SecurityContext)
		applyRawVolumes(pdpValues, products.PingDirectoryProxy.Container)
		applyServiceAnnotations(pdpValues, resolveServiceAnnotations(env.Services, products.PingDirectoryProxy.Service))
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
		"pingdatasync":        pdsValues,
		"pingdirectoryproxy":  pdpValues,
	}

	// Emit top-level volumes map if defined on the environment.
	if len(env.Volumes.Raw) > 0 {
		var vols map[string]any
		if err := json.Unmarshal(env.Volumes.Raw, &vols); err == nil {
			values["volumes"] = vols
		}
	}

	// Emit global.includeVolumes — mounts named volumes into every product's workload.
	if len(env.IncludeVolumes) > 0 {
		globalValues["includeVolumes"] = env.IncludeVolumes
	}

	// Merge spec.envs into global.envs so user-supplied vars are applied to every product.
	if len(env.Envs) > 0 {
		globalEnvs := globalValues["envs"].(map[string]any)
		for k, v := range env.Envs {
			globalEnvs[k] = v
		}
	}

	var err error

	// Merge PingFederate ValuesOverride
	if products.PingFederate != nil {
		values, err = MergeValues(values, products.PingFederate.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingFederate valuesOverride: %w", err)
		}
	}

	// Merge PingDirectory ValuesOverride
	if products.PingDirectory != nil {
		values, err = MergeValues(values, products.PingDirectory.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingDirectory valuesOverride: %w", err)
		}
	}

	// Merge PingAccess ValuesOverride
	if products.PingAccess != nil {
		values, err = MergeValues(values, products.PingAccess.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAccess valuesOverride: %w", err)
		}
	}

	// Merge PingAuthorize ValuesOverride
	if products.PingAuthorize != nil {
		values, err = MergeValues(values, products.PingAuthorize.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAuthorize valuesOverride: %w", err)
		}
	}

	// Merge PingAuthorizePAP ValuesOverride
	if products.PingAuthorizePAP != nil {
		values, err = MergeValues(values, products.PingAuthorizePAP.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingAuthorizePAP valuesOverride: %w", err)
		}
	}

	// Merge PingDataSync ValuesOverride
	if products.PingDataSync != nil {
		values, err = MergeValues(values, products.PingDataSync.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingDataSync valuesOverride: %w", err)
		}
	}

	// Merge PingDirectoryProxy ValuesOverride
	if products.PingDirectoryProxy != nil {
		values, err = MergeValues(values, products.PingDirectoryProxy.ValuesOverride)
		if err != nil {
			return nil, fmt.Errorf("merge PingDirectoryProxy valuesOverride: %w", err)
		}
	}

	return values, nil
}

// resolveWaitForKey maps a logical Ping product name to the ping-devops Helm sub-chart key.
func resolveWaitForKey(application string) string {
	switch strings.ToLower(application) {
	case "pingdirectory":
		return "pingdirectory"
	case "pingfederate", "pingfederateengine":
		return "pingfederate-engine"
	case "pingfederateadmin":
		return "pingfederate-admin"
	case "pingaccess", "pingaccessengine":
		return "pingaccess-engine"
	case "pingaccessadmin":
		return "pingaccess-admin"
	case "pingauthorize":
		return "pingauthorize"
	case "pingauthorizepap":
		return "pingauthorizepap"
	case "pingdataconsole":
		return "pingdataconsole"
	case "pingdatasync":
		return "pingdatasync"
	case "pingdirectoryproxy":
		return "pingdirectoryproxy"
	default:
		return strings.ToLower(application)
	}
}

// buildContainerValues constructs the container map, merging resource requests with
// any waitFor dependencies. Returns an empty map when there is nothing to set.
// Per-product resources and container-level securityContext override the tier defaults.
func buildContainerValues(cpu, memory string, container pingonev1alpha1.ContainerSpec) map[string]any {
	m := map[string]any{}
	if container.Resources != nil {
		m["resources"] = buildResourcesMap(container.Resources)
	} else if cpu != "" || memory != "" {
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
		for _, w := range container.WaitFor {
			entry := map[string]any{"service": w.Service}
			if w.TimeoutSeconds > 0 {
				entry["timeoutSeconds"] = w.TimeoutSeconds
			}
			wf[resolveWaitForKey(w.Application)] = entry
		}
		m["waitFor"] = wf
	}
	if len(container.IncludeVolumes) > 0 {
		m["includeVolumes"] = container.IncludeVolumes
	}
	if csc := rawToMap(container.ContainerSecurityContext); csc != nil {
		m["securityContext"] = csc
	}
	return m
}

// buildResourcesMap converts a ResourceRequirementsSpec into the Helm resources map.
func buildResourcesMap(r *pingonev1alpha1.ResourceRequirementsSpec) map[string]any {
	m := map[string]any{}
	if len(r.Requests) > 0 {
		req := make(map[string]any, len(r.Requests))
		for k, v := range r.Requests {
			req[k] = v
		}
		m["requests"] = req
	}
	if len(r.Limits) > 0 {
		lim := make(map[string]any, len(r.Limits))
		for k, v := range r.Limits {
			lim[k] = v
		}
		m["limits"] = lim
	}
	return m
}

// rawToMap unmarshals a RawExtension into a map. Returns nil on failure or empty input.
func rawToMap(r *runtime.RawExtension) map[string]any {
	if r == nil || len(r.Raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r.Raw, &m); err != nil {
		return nil
	}
	return m
}

// applyRawVolumes passes Kubernetes-native volume/volumeMount arrays directly to the
// ping-devops chart.  The chart renders $v.volumes and $v.volumeMounts with toYaml,
// so standard Kubernetes array format is the expected input.
func applyRawVolumes(productValues map[string]any, container pingonev1alpha1.ContainerSpec) {
	if len(container.Volumes) > 0 {
		vols := make([]any, 0, len(container.Volumes))
		for _, raw := range container.Volumes {
			if len(raw.Raw) == 0 {
				continue
			}
			var v any
			if err := json.Unmarshal(raw.Raw, &v); err != nil {
				continue
			}
			vols = append(vols, v)
		}
		if len(vols) > 0 {
			productValues["volumes"] = vols
		}
	}

	if len(container.VolumeMounts) > 0 {
		mounts := make([]any, 0, len(container.VolumeMounts))
		for _, raw := range container.VolumeMounts {
			if len(raw.Raw) == 0 {
				continue
			}
			var v any
			if err := json.Unmarshal(raw.Raw, &v); err != nil {
				continue
			}
			mounts = append(mounts, v)
		}
		if len(mounts) > 0 {
			productValues["volumeMounts"] = mounts
		}
	}
}

// applyWorkloadSecurityContext merges a pod-level securityContext into a product's workload map.
func applyWorkloadSecurityContext(productValues map[string]any, sc *runtime.RawExtension) {
	m := rawToMap(sc)
	if m == nil {
		return
	}
	wl, ok := productValues["workload"].(map[string]any)
	if !ok {
		wl = map[string]any{}
	}
	wl["securityContext"] = m
	productValues["workload"] = wl
}

// emitServerProfileEnvs writes SERVER_PROFILE_* env vars from a layered profile spec.
// The base profile maps to SERVER_PROFILE_URL/_BRANCH/_PATH/_PARENT.
// Each layer maps to SERVER_PROFILE_<UPPER(layer.Name)>_URL etc.
func emitServerProfileEnvs(envs map[string]any, profile *pingonev1alpha1.ServerProfileSpec, layers []pingonev1alpha1.ServerProfileLayerSpec) {
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
	for _, layer := range layers {
		k := strings.ToUpper(layer.Name)
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

// resolveServiceAnnotations merges global service annotations with per-product ones.
// Product annotations take precedence on conflict.
func resolveServiceAnnotations(global pingonev1alpha1.GlobalServicesSpec, product pingonev1alpha1.ServiceSpec) map[string]string {
	if len(global.Annotations) == 0 {
		return product.Annotations
	}
	merged := make(map[string]string, len(global.Annotations)+len(product.Annotations))
	for k, v := range global.Annotations {
		merged[k] = v
	}
	for k, v := range product.Annotations {
		merged[k] = v
	}
	return merged
}

// applyServiceAnnotations sets services.annotations on a product values map.
func applyServiceAnnotations(productValues map[string]any, annotations map[string]string) {
	if len(annotations) == 0 {
		return
	}
	svcs, _ := productValues["services"].(map[string]any)
	if svcs == nil {
		svcs = make(map[string]any)
	}
	svcs["annotations"] = annotations
	productValues["services"] = svcs
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

// secretEnvFrom returns a container.envFrom list entry referencing a Kubernetes Secret.
func secretEnvFrom(name string) map[string]any {
	return map[string]any{"secretRef": map[string]any{"name": name}}
}

// configMapEnvFrom returns a container.envFrom list entry referencing a Kubernetes ConfigMap.
func configMapEnvFrom(name string) map[string]any {
	return map[string]any{"configMapRef": map[string]any{"name": name}}
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
