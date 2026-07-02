package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// Package-level state shared by the envtest-backed tests. k8sClient stays nil
// when envtest binaries are unavailable (KUBEBUILDER_ASSETS unset); tests that
// need the API server call requireEnv(t) and skip in that case, so a plain
// `go test ./...` still passes without the envtest toolchain. `make test`
// provides the binaries and runs everything.
var (
	k8sClient     client.Client
	envReconciler *PingEnvironmentReconciler
)

// testProducts mirrors the product table in main.go: one entry per product CR
// kind, driving both the field indexes and the shared controller registration.
var testProducts = []struct {
	kind string
	new  func() ProductObject
}{
	{"PingFederate", func() ProductObject { return &pingonev1alpha1.PingFederate{} }},
	{"PingDirectory", func() ProductObject { return &pingonev1alpha1.PingDirectory{} }},
	{"PingAccess", func() ProductObject { return &pingonev1alpha1.PingAccess{} }},
	{"PingAuthorize", func() ProductObject { return &pingonev1alpha1.PingAuthorize{} }},
	{"PingAuthorizePAP", func() ProductObject { return &pingonev1alpha1.PingAuthorizePAP{} }},
	{"PingDataSync", func() ProductObject { return &pingonev1alpha1.PingDataSync{} }},
	{"PingDirectoryProxy", func() ProductObject { return &pingonev1alpha1.PingDirectoryProxy{} }},
}

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "KUBEBUILDER_ASSETS not set; skipping envtest-backed controller tests (run via `make test`)")
		os.Exit(m.Run())
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start envtest: %v\n", err)
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "add client-go scheme: %v\n", err)
		os.Exit(1)
	}
	if err := pingonev1alpha1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "add pingone scheme: %v\n", err)
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create manager: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Same environmentRef indexes and product controllers as main.go. The
	// PingEnvironment reconciler is NOT started: its Reconcile drives Helm
	// (chart download + release install), which envtest cannot support. Its
	// pure pieces (listProducts, enqueueFromEnvironmentRef) are tested by
	// calling them directly on envReconciler.
	for _, p := range testProducts {
		if err := mgr.GetFieldIndexer().IndexField(ctx, p.new(), "spec.environmentRef", func(o client.Object) []string {
			return []string{o.(ProductObject).GetEnvironmentRef()}
		}); err != nil {
			fmt.Fprintf(os.Stderr, "index %s: %v\n", p.kind, err)
			os.Exit(1)
		}
		if err := (&ProductReconciler{Client: mgr.GetClient(), NewObject: p.new, Kind: p.kind}).SetupWithManager(mgr); err != nil {
			fmt.Fprintf(os.Stderr, "setup %s controller: %v\n", p.kind, err)
			os.Exit(1)
		}
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "manager exited: %v\n", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		fmt.Fprintln(os.Stderr, "cache never synced")
		os.Exit(1)
	}

	k8sClient = mgr.GetClient()
	envReconciler = &PingEnvironmentReconciler{Client: mgr.GetClient(), Scheme: scheme}

	code := m.Run()

	cancel()
	if err := testEnv.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop envtest: %v\n", err)
	}
	os.Exit(code)
}

// requireEnv skips the test when the envtest API server is not running.
func requireEnv(t *testing.T) {
	t.Helper()
	if k8sClient == nil {
		t.Skip("envtest not available (KUBEBUILDER_ASSETS unset); run via `make test`")
	}
}

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

// holdsFor asserts cond stays true for the whole window (polling), for
// "nothing should happen" style checks.
func holdsFor(t *testing.T, window time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("condition %q violated within %s window", what, window)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
