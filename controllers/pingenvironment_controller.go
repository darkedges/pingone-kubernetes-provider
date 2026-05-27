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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
	helmclient "github.com/darkedges/pingone-operator/internal/helm"
)

const (
	helmRepoURL   = "https://helm.pingidentity.com/"
	helmChartName = "ping-devops"
	helmChartVer  = "0.12.2"
	helmCacheDir  = "/tmp/helm-cache"
)

// productCRLists holds the CRs found for each product kind in a given environment.
type productCRLists struct {
	PingFederate     []pingonev1alpha1.PingFederate
	PingDirectory    []pingonev1alpha1.PingDirectory
	PingAccess       []pingonev1alpha1.PingAccess
	PingAuthorize    []pingonev1alpha1.PingAuthorize
	PingAuthorizePAP []pingonev1alpha1.PingAuthorizePAP
}

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

// SetupWithManager registers the reconciler and watches all product CRDs.
func (r *PingEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingEnvironment{}).
		Watches(&pingonev1alpha1.PingFederate{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingDirectory{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingAccess{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingAuthorize{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingAuthorizePAP{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Complete(r)
}

// enqueueFromEnvironmentRef extracts the PingEnvironment name from a product CR and enqueues it.
func enqueueFromEnvironmentRef(_ context.Context, obj client.Object) []reconcile.Request {
	var ref string
	switch o := obj.(type) {
	case *pingonev1alpha1.PingFederate:
		ref = o.Spec.EnvironmentRef
	case *pingonev1alpha1.PingDirectory:
		ref = o.Spec.EnvironmentRef
	case *pingonev1alpha1.PingAccess:
		ref = o.Spec.EnvironmentRef
	case *pingonev1alpha1.PingAuthorize:
		ref = o.Spec.EnvironmentRef
	case *pingonev1alpha1.PingAuthorizePAP:
		ref = o.Spec.EnvironmentRef
	}
	if ref == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: ref, Namespace: obj.GetNamespace()}}}
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

	if !containsFinalizer(env.Finalizers, finalizerName) {
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

	products, lists, err := r.listProducts(ctx, env)
	if err != nil {
		return ctrl.Result{}, err
	}

	releaseName := fmt.Sprintf("%s-ping", env.Spec.TenantID)

	if err := r.deployPingDevops(cfg, env, releaseName, targetNS, products); err != nil {
		logger.Error(err, "failed to deploy ping-devops")
		return r.setFailed(ctx, env, lists, "DeployFailed", err.Error())
	}

	return r.setReady(ctx, env, releaseName, lists)
}

// listProducts queries all product CRs that reference this environment and builds ProductSpecs.
func (r *PingEnvironmentReconciler) listProducts(ctx context.Context, env *pingonev1alpha1.PingEnvironment) (helmclient.ProductSpecs, productCRLists, error) {
	var (
		specs helmclient.ProductSpecs
		lists productCRLists
	)
	fieldSel := client.MatchingFields{"spec.environmentRef": env.Name}
	ns := client.InNamespace(env.Namespace)

	pfList := &pingonev1alpha1.PingFederateList{}
	if err := r.List(ctx, pfList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingFederates: %w", err)
	}
	if len(pfList.Items) > 0 {
		lists.PingFederate = pfList.Items
		specs.PingFederate = &pfList.Items[0].Spec
	}

	pdList := &pingonev1alpha1.PingDirectoryList{}
	if err := r.List(ctx, pdList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingDirectories: %w", err)
	}
	if len(pdList.Items) > 0 {
		lists.PingDirectory = pdList.Items
		specs.PingDirectory = &pdList.Items[0].Spec
	}

	paList := &pingonev1alpha1.PingAccessList{}
	if err := r.List(ctx, paList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAccesses: %w", err)
	}
	if len(paList.Items) > 0 {
		lists.PingAccess = paList.Items
		specs.PingAccess = &paList.Items[0].Spec
	}

	pazList := &pingonev1alpha1.PingAuthorizeList{}
	if err := r.List(ctx, pazList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAuthorizes: %w", err)
	}
	if len(pazList.Items) > 0 {
		lists.PingAuthorize = pazList.Items
		specs.PingAuthorize = &pazList.Items[0].Spec
	}

	papList := &pingonev1alpha1.PingAuthorizePAPList{}
	if err := r.List(ctx, papList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAuthorizePAPs: %w", err)
	}
	if len(papList.Items) > 0 {
		lists.PingAuthorizePAP = papList.Items
		specs.PingAuthorizePAP = &papList.Items[0].Spec
	}

	return specs, lists, nil
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

func (r *PingEnvironmentReconciler) deployPingDevops(cfg *action.Configuration, env *pingonev1alpha1.PingEnvironment, releaseName, namespace string, products helmclient.ProductSpecs) error {
	values, err := helmclient.BuildPingValues(env.Spec, products)
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

	return helmclient.InstallOrUpgrade(cfg, releaseName, namespace, ch, values, true)
}

func (r *PingEnvironmentReconciler) setReady(ctx context.Context, env *pingonev1alpha1.PingEnvironment, releaseName string, lists productCRLists) (ctrl.Result, error) {
	patch := client.MergeFrom(env.DeepCopy())
	env.Status.Phase = "Ready"
	env.Status.Release = releaseName
	env.Status.ObservedGeneration = env.Generation
	apimeta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Deployed",
		Message:            "All components deployed successfully",
		ObservedGeneration: env.Generation,
	})
	if err := r.Status().Patch(ctx, env, patch); err != nil {
		return ctrl.Result{}, err
	}
	r.updateProductStatuses(ctx, releaseName, "Ready", lists)
	return ctrl.Result{}, nil
}

func (r *PingEnvironmentReconciler) setFailed(ctx context.Context, env *pingonev1alpha1.PingEnvironment, lists productCRLists, reason, msg string) (ctrl.Result, error) {
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
	r.updateProductStatuses(ctx, "", "Failed", lists)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PingEnvironmentReconciler) updateProductStatuses(ctx context.Context, releaseName, phase string, lists productCRLists) {
	for i := range lists.PingFederate {
		pf := &lists.PingFederate[i]
		patch := client.MergeFrom(pf.DeepCopy())
		pf.Status.Phase = phase
		pf.Status.Release = releaseName
		_ = r.Status().Patch(ctx, pf, patch)
	}
	for i := range lists.PingDirectory {
		pd := &lists.PingDirectory[i]
		patch := client.MergeFrom(pd.DeepCopy())
		pd.Status.Phase = phase
		pd.Status.Release = releaseName
		_ = r.Status().Patch(ctx, pd, patch)
	}
	for i := range lists.PingAccess {
		pa := &lists.PingAccess[i]
		patch := client.MergeFrom(pa.DeepCopy())
		pa.Status.Phase = phase
		pa.Status.Release = releaseName
		_ = r.Status().Patch(ctx, pa, patch)
	}
	for i := range lists.PingAuthorize {
		paz := &lists.PingAuthorize[i]
		patch := client.MergeFrom(paz.DeepCopy())
		paz.Status.Phase = phase
		paz.Status.Release = releaseName
		_ = r.Status().Patch(ctx, paz, patch)
	}
	for i := range lists.PingAuthorizePAP {
		pap := &lists.PingAuthorizePAP[i]
		patch := client.MergeFrom(pap.DeepCopy())
		pap.Status.Phase = phase
		pap.Status.Release = releaseName
		_ = r.Status().Patch(ctx, pap, patch)
	}
}
