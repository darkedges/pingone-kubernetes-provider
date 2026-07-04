// main is the entry point for the PingOne Kubernetes Operator.
package main

import (
	"context"
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
	"github.com/darkedges/pingone-operator/controllers"
)

// Set at build time via -ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(pingonev1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Address for the metrics endpoint.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Address for the health probe endpoint.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for high availability.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("starting operator", "version", version, "commit", commit, "buildDate", buildDate)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "pingone-operator-leader-election",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// One entry per product CR kind: drives both the environmentRef field index
	// and the shared product controller registration.
	products := []struct {
		kind string
		new  func() controllers.ProductObject
	}{
		{"PingFederate", func() controllers.ProductObject { return &pingonev1alpha1.PingFederate{} }},
		{"PingDirectory", func() controllers.ProductObject { return &pingonev1alpha1.PingDirectory{} }},
		{"PingAccess", func() controllers.ProductObject { return &pingonev1alpha1.PingAccess{} }},
		{"PingAuthorize", func() controllers.ProductObject { return &pingonev1alpha1.PingAuthorize{} }},
		{"PingAuthorizePAP", func() controllers.ProductObject { return &pingonev1alpha1.PingAuthorizePAP{} }},
		{"PingDataSync", func() controllers.ProductObject { return &pingonev1alpha1.PingDataSync{} }},
		{"PingDirectoryProxy", func() controllers.ProductObject { return &pingonev1alpha1.PingDirectoryProxy{} }},
		{"PingDataConsole", func() controllers.ProductObject { return &pingonev1alpha1.PingDataConsole{} }},
	}

	// Register field indexes so product CRs can be listed by environmentRef.
	ctx := context.Background()
	for _, p := range products {
		if err := mgr.GetFieldIndexer().IndexField(ctx, p.new(), "spec.environmentRef", func(o client.Object) []string {
			return []string{o.(controllers.ProductObject).GetEnvironmentRef()}
		}); err != nil {
			setupLog.Error(err, "unable to index field", "kind", p.kind)
			os.Exit(1)
		}
	}

	if err = (&controllers.PingEnvironmentReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RESTConfig: mgr.GetConfig(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PingEnvironment")
		os.Exit(1)
	}

	for _, p := range products {
		if err = (&controllers.ProductReconciler{
			Client:    mgr.GetClient(),
			NewObject: p.new,
			Kind:      p.kind,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", p.kind)
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
