package helm

import (
	"encoding/json"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/runtime"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// mustRaw encodes v as JSON into a runtime.RawExtension, panicking on error.
func mustRaw(v any) runtime.RawExtension {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{Raw: b}
}

// ---- applyRawVolumes -------------------------------------------------------

// Regression: the previous implementation converted the user-supplied Kubernetes
// array format into a ping-devops map format.  The chart renders $v.volumes with
// toYaml and expects a standard Kubernetes array, so the conversion produced
// unknown field warnings (volume names appeared as keys on spec.template.spec
// instead of entries in spec.template.spec.volumes).
func TestApplyRawVolumes_ProducesArrayNotMap(t *testing.T) {
	container := pingonev1alpha1.ContainerSpec{
		Volumes: []runtime.RawExtension{
			mustRaw(map[string]any{
				"name":   "github-secret",
				"secret": map[string]any{"secretName": "github-secret"},
			}),
		},
		VolumeMounts: []runtime.RawExtension{
			mustRaw(map[string]any{
				"name":      "github-secret",
				"mountPath": "/secrets/github",
			}),
		},
	}

	productValues := map[string]any{}
	applyRawVolumes(productValues, container)

	// Must be a slice, not a map.
	vols, ok := productValues["volumes"].([]any)
	if !ok {
		t.Fatalf("productValues[\"volumes\"] type = %T, want []any (array); got %v", productValues["volumes"], productValues["volumes"])
	}
	if len(vols) != 1 {
		t.Fatalf("volumes len = %d, want 1", len(vols))
	}

	mounts, ok := productValues["volumeMounts"].([]any)
	if !ok {
		t.Fatalf("productValues[\"volumeMounts\"] type = %T, want []any (array)", productValues["volumeMounts"])
	}
	if len(mounts) != 1 {
		t.Fatalf("volumeMounts len = %d, want 1", len(mounts))
	}
}

// Each volume entry must preserve its "name" field (the broken implementation
// deleted it when converting to a map).
func TestApplyRawVolumes_PreservesNameField(t *testing.T) {
	container := pingonev1alpha1.ContainerSpec{
		Volumes: []runtime.RawExtension{
			mustRaw(map[string]any{
				"name":      "app-config",
				"configMap": map[string]any{"name": "app-config"},
			}),
		},
	}

	productValues := map[string]any{}
	applyRawVolumes(productValues, container)

	vols := productValues["volumes"].([]any)
	entry := vols[0].(map[string]any)
	if entry["name"] != "app-config" {
		t.Errorf("volume name = %q, want \"app-config\"", entry["name"])
	}
}

// Multiple volumes and mounts must all appear in the output arrays.
func TestApplyRawVolumes_MultipleEntries(t *testing.T) {
	container := pingonev1alpha1.ContainerSpec{
		Volumes: []runtime.RawExtension{
			mustRaw(map[string]any{"name": "secret-vol", "secret": map[string]any{"secretName": "my-secret"}}),
			mustRaw(map[string]any{"name": "cm-vol", "configMap": map[string]any{"name": "my-cm"}}),
		},
		VolumeMounts: []runtime.RawExtension{
			mustRaw(map[string]any{"name": "secret-vol", "mountPath": "/secrets"}),
			mustRaw(map[string]any{"name": "cm-vol", "mountPath": "/config"}),
		},
	}

	productValues := map[string]any{}
	applyRawVolumes(productValues, container)

	if got := len(productValues["volumes"].([]any)); got != 2 {
		t.Errorf("volumes count = %d, want 2", got)
	}
	if got := len(productValues["volumeMounts"].([]any)); got != 2 {
		t.Errorf("volumeMounts count = %d, want 2", got)
	}
}

// When the container has no volumes, nothing should be written to productValues.
func TestApplyRawVolumes_Empty(t *testing.T) {
	productValues := map[string]any{}
	applyRawVolumes(productValues, pingonev1alpha1.ContainerSpec{})

	if _, exists := productValues["volumes"]; exists {
		t.Error("expected no \"volumes\" key when container has no volumes")
	}
	if _, exists := productValues["volumeMounts"]; exists {
		t.Error("expected no \"volumeMounts\" key when container has no volumeMounts")
	}
}

// A volumes list without a corresponding volumeMounts list is valid.
func TestApplyRawVolumes_VolumesOnlyNoMounts(t *testing.T) {
	container := pingonev1alpha1.ContainerSpec{
		Volumes: []runtime.RawExtension{
			mustRaw(map[string]any{"name": "empty-dir", "emptyDir": map[string]any{}}),
		},
	}

	productValues := map[string]any{}
	applyRawVolumes(productValues, container)

	if _, exists := productValues["volumes"]; !exists {
		t.Error("expected \"volumes\" key")
	}
	if _, exists := productValues["volumeMounts"]; exists {
		t.Error("expected no \"volumeMounts\" key when container.VolumeMounts is empty")
	}
}

// An entry with a nil Raw field must be silently skipped; the remaining entries
// must still appear.
func TestApplyRawVolumes_SkipsNilRaw(t *testing.T) {
	container := pingonev1alpha1.ContainerSpec{
		Volumes: []runtime.RawExtension{
			{Raw: nil},
			mustRaw(map[string]any{"name": "good-vol", "emptyDir": map[string]any{}}),
		},
	}

	productValues := map[string]any{}
	applyRawVolumes(productValues, container)

	vols := productValues["volumes"].([]any)
	if len(vols) != 1 {
		t.Errorf("volumes len = %d, want 1 (nil entry should be skipped)", len(vols))
	}
}

// ---- resolveServiceAnnotations --------------------------------------------

func TestResolveServiceAnnotations_GlobalOnly(t *testing.T) {
	global := pingonev1alpha1.GlobalServicesSpec{
		Annotations: map[string]string{"konghq.com/protocol": "https"},
	}
	got := resolveServiceAnnotations(global, pingonev1alpha1.ServiceSpec{})
	if got["konghq.com/protocol"] != "https" {
		t.Errorf("annotation missing or wrong: %v", got)
	}
}

func TestResolveServiceAnnotations_ProductOverridesGlobal(t *testing.T) {
	global := pingonev1alpha1.GlobalServicesSpec{
		Annotations: map[string]string{"konghq.com/protocol": "https", "shared": "global"},
	}
	product := pingonev1alpha1.ServiceSpec{
		Annotations: map[string]string{"konghq.com/protocol": "http", "product": "only"},
	}
	got := resolveServiceAnnotations(global, product)
	// product wins on conflict
	if got["konghq.com/protocol"] != "http" {
		t.Errorf("product annotation should override global, got %q", got["konghq.com/protocol"])
	}
	// global key not overridden by product is preserved
	if got["shared"] != "global" {
		t.Errorf("global-only key lost: %v", got)
	}
	// product-only key is preserved
	if got["product"] != "only" {
		t.Errorf("product-only key lost: %v", got)
	}
}

func TestResolveServiceAnnotations_NeitherSet(t *testing.T) {
	got := resolveServiceAnnotations(
		pingonev1alpha1.GlobalServicesSpec{},
		pingonev1alpha1.ServiceSpec{},
	)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// ---- applyServiceAnnotations ----------------------------------------------

func TestApplyServiceAnnotations_AddsToExistingServicesMap(t *testing.T) {
	productValues := map[string]any{
		"services": map[string]any{
			"https": map[string]any{"containerPort": 9031},
		},
	}
	applyServiceAnnotations(productValues, map[string]string{"konghq.com/protocol": "https"})

	svcs := productValues["services"].(map[string]any)
	ann, ok := svcs["annotations"].(map[string]string)
	if !ok {
		t.Fatalf("services.annotations type = %T, want map[string]string", svcs["annotations"])
	}
	if ann["konghq.com/protocol"] != "https" {
		t.Errorf("annotation not set: %v", ann)
	}
	// existing service entry must survive
	if svcs["https"] == nil {
		t.Error("existing 'https' service entry was lost")
	}
}

func TestApplyServiceAnnotations_CreatesServicesMapWhenAbsent(t *testing.T) {
	productValues := map[string]any{}
	applyServiceAnnotations(productValues, map[string]string{"konghq.com/protocol": "https"})

	svcs, ok := productValues["services"].(map[string]any)
	if !ok {
		t.Fatalf("services type = %T, want map[string]any", productValues["services"])
	}
	if svcs["annotations"] == nil {
		t.Error("services.annotations not set")
	}
}

func TestApplyServiceAnnotations_NoOpWhenEmpty(t *testing.T) {
	productValues := map[string]any{}
	applyServiceAnnotations(productValues, nil)
	applyServiceAnnotations(productValues, map[string]string{})
	if _, exists := productValues["services"]; exists {
		t.Error("services key should not be created when annotations are empty")
	}
}

// ---- MergeValues -----------------------------------------------------------

func TestMergeValues_EmptyOverride(t *testing.T) {
	base := map[string]any{"key": "value"}
	result, err := MergeValues(base, runtime.RawExtension{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("base key lost after empty override merge")
	}
}

func TestMergeValues_OverrideWins(t *testing.T) {
	base := map[string]any{"replicas": 1}
	override := mustRaw(map[string]any{"replicas": 3})
	result, err := MergeValues(base, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JSON numbers unmarshal to float64.
	if result["replicas"] != float64(3) {
		t.Errorf("replicas = %v, want 3", result["replicas"])
	}
}

func TestMergeValues_AddsNewKeys(t *testing.T) {
	base := map[string]any{"a": 1}
	override := mustRaw(map[string]any{"b": 2})
	result, err := MergeValues(base, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["a"] == nil {
		t.Error("base key \"a\" was lost")
	}
	if result["b"] == nil {
		t.Error("override key \"b\" not present")
	}
}

// ---- buildImageValues -------------------------------------------------------

func TestBuildImageValues(t *testing.T) {
	for _, tc := range []struct {
		name, repository, version string
		want                      map[string]any
	}{
		{
			name: "bare tag", version: "13.0.2-edge",
			want: map[string]any{"tag": "13.0.2-edge"},
		},
		{
			name: "full reference", version: "docker.io/pingidentity/pingfederate:13.0.2-edge",
			want: map[string]any{"repository": "docker.io/pingidentity", "name": "pingfederate", "tag": "13.0.2-edge"},
		},
		{
			name: "registry with port and tag", version: "registry.example.com:5000/pingfederate:1.2.3",
			want: map[string]any{"repository": "registry.example.com:5000", "name": "pingfederate", "tag": "1.2.3"},
		},
		{
			// Regression: the port colon must not be mistaken for a tag separator.
			name: "registry with port, no tag", version: "registry.example.com:5000/pingfederate",
			want: map[string]any{"repository": "registry.example.com:5000", "name": "pingfederate"},
		},
		{
			name: "explicit repository overrides parsed", repository: "custom.repo",
			version: "docker.io/pingidentity/pingfederate:1.2.3",
			want:    map[string]any{"repository": "custom.repo", "name": "pingfederate", "tag": "1.2.3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildImageValues(tc.repository, tc.version)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

// ---- deepCopyValues ---------------------------------------------------------

func TestDeepCopyValues_NoAliasing(t *testing.T) {
	orig := map[string]any{
		"envs":    map[string]any{"KEY": "one"},
		"envFrom": []map[string]any{{"secretRef": map[string]any{"name": "s1"}}},
		"labels":  map[string]string{"app": "pf"},
		"list":    []any{map[string]any{"k": "v"}},
	}
	cp := deepCopyValues(orig)

	cp["envs"].(map[string]any)["KEY"] = "two"
	cp["envFrom"].([]map[string]any)[0]["secretRef"].(map[string]any)["name"] = "s2"
	cp["labels"].(map[string]string)["app"] = "pa"
	cp["list"].([]any)[0].(map[string]any)["k"] = "changed"

	if orig["envs"].(map[string]any)["KEY"] != "one" {
		t.Error("nested map aliased: mutation through copy changed original")
	}
	if orig["envFrom"].([]map[string]any)[0]["secretRef"].(map[string]any)["name"] != "s1" {
		t.Error("[]map[string]any aliased")
	}
	if orig["labels"].(map[string]string)["app"] != "pf" {
		t.Error("map[string]string aliased")
	}
	if orig["list"].([]any)[0].(map[string]any)["k"] != "v" {
		t.Error("[]any element aliased")
	}
}

// ---- SERVER_PROFILE_URL mapping -------------------------------------------

func TestBuildPingValues_PingDirectory_ServerProfileURL(t *testing.T) {
	env := pingonev1alpha1.PingEnvironmentSpec{
		TenantID: "test",
		Tier:     "development",
	}
	products := ProductSpecs{
		PingDirectory: &pingonev1alpha1.PingDirectorySpec{
			Config: pingonev1alpha1.PingDirectoryConfig{
				ServerProfile: &pingonev1alpha1.ServerProfileSpec{
					URL:    "https://github.com/your-org/ping-profiles.git",
					Branch: "main",
					Path:   "pingdirectory",
				},
				UserBaseDN: "dc=example,dc=com",
			},
		},
	}

	vals, err := BuildPingValues(env, products)
	if err != nil {
		t.Fatalf("BuildPingValues error: %v", err)
	}

	pd, ok := vals["pingdirectory"].(map[string]any)
	if !ok {
		t.Fatal("pingdirectory key missing or wrong type")
	}
	envs, ok := pd["envs"].(map[string]any)
	if !ok {
		t.Fatal("pingdirectory.envs missing or wrong type")
	}

	for _, tc := range []struct{ key, want string }{
		{"SERVER_PROFILE_URL", "https://github.com/your-org/ping-profiles.git"},
		{"SERVER_PROFILE_BRANCH", "main"},
		{"SERVER_PROFILE_PATH", "pingdirectory"},
	} {
		got, ok := envs[tc.key]
		if !ok {
			t.Errorf("envs[%q] not set", tc.key)
			continue
		}
		if got != tc.want {
			t.Errorf("envs[%q] = %q, want %q", tc.key, got, tc.want)
		}
	}
}

// ---- releaseUnchanged / jsonEqual ------------------------------------------

func TestJSONEqual(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b map[string]any
		want bool
	}{
		{
			// Helm's storage round-trip turns int32 into float64 and []string
			// into []interface{}; jsonEqual must treat those as identical.
			name: "helm storage type round-trip",
			a:    map[string]any{"replicas": int32(3), "hosts": []string{"a", "b"}},
			b:    map[string]any{"replicas": float64(3), "hosts": []any{"a", "b"}},
			want: true,
		},
		{
			name: "key order irrelevant",
			a:    map[string]any{"x": 1, "y": 2},
			b:    map[string]any{"y": 2, "x": 1},
			want: true,
		},
		{
			name: "differing values",
			a:    map[string]any{"replicas": 3},
			b:    map[string]any{"replicas": 4},
			want: false,
		},
		{
			name: "missing key",
			a:    map[string]any{"x": 1, "y": 2},
			b:    map[string]any{"x": 1},
			want: false,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "unmarshalable value",
			a:    map[string]any{"bad": func() {}},
			b:    map[string]any{"bad": func() {}},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("jsonEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReleaseUnchanged(t *testing.T) {
	values := map[string]any{"pingfederate": map[string]any{"enabled": true, "replicas": int32(2)}}
	// Simulate Helm's storage round-trip of the same values.
	storedValues := map[string]any{"pingfederate": map[string]any{"enabled": true, "replicas": float64(2)}}

	newChart := func(version string) *chart.Chart {
		return &chart.Chart{Metadata: &chart.Metadata{Name: "ping-devops", Version: version}}
	}
	newRelease := func(status release.Status, chartVersion string, config map[string]any) *release.Release {
		return &release.Release{
			Info:   &release.Info{Status: status},
			Chart:  newChart(chartVersion),
			Config: config,
		}
	}

	for _, tc := range []struct {
		name string
		rel  *release.Release
		ch   *chart.Chart
		want bool
	}{
		{
			name: "deployed with matching version and values",
			rel:  newRelease(release.StatusDeployed, "0.12.2", storedValues),
			ch:   newChart("0.12.2"),
			want: true,
		},
		{
			name: "values differ",
			rel: newRelease(release.StatusDeployed, "0.12.2",
				map[string]any{"pingfederate": map[string]any{"enabled": true, "replicas": float64(3)}}),
			ch:   newChart("0.12.2"),
			want: false,
		},
		{
			name: "chart version differs",
			rel:  newRelease(release.StatusDeployed, "0.12.1", storedValues),
			ch:   newChart("0.12.2"),
			want: false,
		},
		{
			name: "failed release always upgraded",
			rel:  newRelease(release.StatusFailed, "0.12.2", storedValues),
			ch:   newChart("0.12.2"),
			want: false,
		},
		{
			name: "pending release always upgraded",
			rel:  newRelease(release.StatusPendingUpgrade, "0.12.2", storedValues),
			ch:   newChart("0.12.2"),
			want: false,
		},
		{
			name: "nil release info",
			rel:  &release.Release{Chart: newChart("0.12.2"), Config: storedValues},
			ch:   newChart("0.12.2"),
			want: false,
		},
		{
			name: "nil release chart",
			rel: &release.Release{
				Info:   &release.Info{Status: release.StatusDeployed},
				Config: storedValues,
			},
			ch:   newChart("0.12.2"),
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseUnchanged(tc.rel, tc.ch, values); got != tc.want {
				t.Errorf("releaseUnchanged = %v, want %v", got, tc.want)
			}
		})
	}
}
