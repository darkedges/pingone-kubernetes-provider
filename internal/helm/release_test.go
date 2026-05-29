package helm

import (
	"encoding/json"
	"testing"

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
				"name":   "app-config",
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
