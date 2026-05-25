// Package controllers implements the PingEnvironment reconciler.
package controllers

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
	helmclient "github.com/darkedges/pingone-operator/internal/helm"
)

const (
	finalizerName = "pingone.io/cleanup"
	helmRepoURL   = "https://helm.pingidentity.com/"
	helmChartName = "ping-devops"
	helmChartVer  = "0.12.2"
	helmCacheDir  = "/tmp/helm-cache"
)

// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;configmaps;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// PingEnvironmentReconciler reconciles a PingEnvironment object.
type PingEnvironmentReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RESTConfig *rest.Config
}

// SetupWithManager registers the reconciler with the given controller manager.
func (r *PingEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingEnvironment{}).
		Complete(r)
}

// Reconcile is the main reconciliation loop for PingEnvironment resources.
func (r *PingEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, req.NamespacedName, env); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !env.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, env)
	}

	if !containsFinalizer(env, finalizerName) {
		env.Finalizers = append(env.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, env)
	}

	targetNS := env.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = env.Namespace
	}

	cfg, err := helmclient.NewHelmClient(targetNS, r.RESTConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("helm client: %w", err)
	}

	needsUpgrade := env.Generation != env.Status.ObservedGeneration
	releaseName := fmt.Sprintf("%s-ping", env.Spec.TenantID)

	if err := r.deployPingDevops(ctx, cfg, env, releaseName, targetNS, needsUpgrade); err != nil {
		logger.Error(err, "failed to deploy ping-devops")
		return r.setFailed(ctx, env, "DeployFailed", err.Error())
	}

	return r.setReady(ctx, env, releaseName)
}

func (r *PingEnvironmentReconciler) handleDeletion(ctx context.Context, env *pingonev1alpha1.PingEnvironment) (ctrl.Result, error) {
	targetNS := env.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = env.Namespace
	}
	cfg, err := helmclient.NewHelmClient(targetNS, r.RESTConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("helm client: %w", err)
	}

	releaseName := fmt.Sprintf("%s-ping", env.Spec.TenantID)
	if helmclient.ReleaseExists(cfg, releaseName) {
		if _, err := action.NewUninstall(cfg).Run(releaseName); err != nil {
			return ctrl.Result{}, fmt.Errorf("uninstall %s: %w", releaseName, err)
		}
	}

	env.Finalizers = removeFinalizer(env.Finalizers, finalizerName)
	return ctrl.Result{}, r.Update(ctx, env)
}

func (r *PingEnvironmentReconciler) deployPingDevops(ctx context.Context, cfg *action.Configuration, env *pingonev1alpha1.PingEnvironment, releaseName, namespace string, upgrade bool) error {
	values, err := helmclient.BuildPingValues(env.Spec)
	if err != nil {
		return fmt.Errorf("build ping values: %w", err)
	}

	chartPath, err := helmclient.DownloadChart(helmRepoURL, helmChartName, helmChartVer, helmCacheDir)
	if err != nil {
		return fmt.Errorf("download %s chart: %w", helmChartName, err)
	}

	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("load %s chart: %w", helmChartName, err)
	}

	return helmclient.InstallOrUpgrade(cfg, releaseName, namespace, ch, values, upgrade)
}

func (r *PingEnvironmentReconciler) setReady(ctx context.Context, env *pingonev1alpha1.PingEnvironment, releaseName string) (ctrl.Result, error) {
	patch := client.MergeFrom(env.DeepCopy())
	env.Status.Phase = "Ready"
	env.Status.PingFederateRelease = releaseName
	env.Status.PingDirectoryRelease = releaseName
	if env.Spec.PingAccess != nil {
		env.Status.PingAccessRelease = releaseName
	}
	if env.Spec.PingAuthorize != nil {
		env.Status.PingAuthorizeRelease = releaseName
	}
	if env.Spec.PingAuthorizePAP != nil {
		env.Status.PingAuthorizePAPRelease = releaseName
	}
	env.Status.ObservedGeneration = env.Generation
	apimeta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Deployed",
		Message:            "All components deployed successfully",
		ObservedGeneration: env.Generation,
	})
	return ctrl.Result{}, r.Status().Patch(ctx, env, patch)
}

func (r *PingEnvironmentReconciler) setFailed(ctx context.Context, env *pingonev1alpha1.PingEnvironment, reason, msg string) (ctrl.Result, error) {
	patch := client.MergeFrom(env.DeepCopy())
	env.Status.Phase = "Failed"
	apimeta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: env.Generation,
	})
	if err := r.Status().Patch(ctx, env, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func containsFinalizer(env *pingonev1alpha1.PingEnvironment, finalizer string) bool {
	for _, f := range env.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, finalizer string) []string {
	out := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != finalizer {
			out = append(out, f)
		}
	}
	return out
}
