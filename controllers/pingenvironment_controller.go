// Package controllers implements the PingEnvironment reconciler.
package controllers

import (
	"context"
	"fmt"
	"sort"
	"time"

	"helm.sh/helm/v3/pkg/action"
	helmChartPkg "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	PingFederate       []pingonev1alpha1.PingFederate
	PingDirectory      []pingonev1alpha1.PingDirectory
	PingAccess         []pingonev1alpha1.PingAccess
	PingAuthorize      []pingonev1alpha1.PingAuthorize
	PingAuthorizePAP   []pingonev1alpha1.PingAuthorizePAP
	PingDataSync       []pingonev1alpha1.PingDataSync
	PingDirectoryProxy []pingonev1alpha1.PingDirectoryProxy
	PingDataConsole    []pingonev1alpha1.PingDataConsole
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;configmaps;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// PingEnvironmentReconciler reconciles a PingEnvironment object.
// The cached fields are not mutex-protected: the controller runs with the
// default MaxConcurrentReconciles of 1, so Reconcile is never concurrent.
type PingEnvironmentReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	RESTConfig  *rest.Config
	cachedChart *helmChart
	// helmConfigs caches one action.Configuration per target namespace so the
	// discovery client and REST mapper are not rebuilt on every reconcile.
	helmConfigs map[string]*action.Configuration
}

// helmConfigFor returns a cached Helm action.Configuration for the namespace,
// creating and caching it on first use.
func (r *PingEnvironmentReconciler) helmConfigFor(namespace string) (*action.Configuration, error) {
	if cfg, ok := r.helmConfigs[namespace]; ok {
		return cfg, nil
	}
	cfg, err := helmclient.NewHelmClient(namespace, r.RESTConfig)
	if err != nil {
		return nil, err
	}
	if r.helmConfigs == nil {
		r.helmConfigs = make(map[string]*action.Configuration)
	}
	r.helmConfigs[namespace] = cfg
	return cfg, nil
}

// helmChart holds the loaded chart and the path it was loaded from, so we only
// call loader.Load once per operator process rather than on every reconcile.
type helmChart struct {
	path  string
	chart *helmChartPkg.Chart
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
		Watches(&pingonev1alpha1.PingDataSync{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingDirectoryProxy{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Watches(&pingonev1alpha1.PingDataConsole{}, handler.EnqueueRequestsFromMapFunc(enqueueFromEnvironmentRef)).
		Complete(r)
}

// enqueueFromEnvironmentRef extracts the PingEnvironment name from a product CR and enqueues it.
func enqueueFromEnvironmentRef(_ context.Context, obj client.Object) []reconcile.Request {
	po, ok := obj.(ProductObject)
	if !ok {
		return nil
	}
	ref := po.GetEnvironmentRef()
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

	if controllerutil.AddFinalizer(env, finalizerName) {
		return ctrl.Result{Requeue: true}, r.Update(ctx, env)
	}

	targetNS := env.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = env.Namespace
	}

	cfg, err := r.helmConfigFor(targetNS)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("helm client: %w", err)
	}

	products, lists, err := r.listProducts(ctx, env)
	if err != nil {
		return ctrl.Result{}, err
	}

	releaseName := fmt.Sprintf("%s-ping", env.Spec.TenantID)

	helmAction, err := r.deployPingDevops(cfg, env, releaseName, targetNS, products)
	if err != nil {
		logger.Error(err, "failed to deploy ping-devops")
		return r.setFailed(ctx, env, releaseName, lists, "DeployFailed", err.Error())
	}
	if helmAction == helmclient.ActionUnchanged {
		logger.V(1).Info("helm release unchanged; upgrade skipped", "release", releaseName)
	} else {
		logger.Info("reconciled helm release", "release", releaseName, "action", helmAction)
	}

	return r.setReady(ctx, env, releaseName, lists)
}

// listProducts queries all product CRs that reference this environment and builds ProductSpecs.
// Each list is sorted by name before the first item's spec is chosen: cache list
// order is not guaranteed, so without sorting the winning CR could change between
// reconciles when several CRs of one kind reference the same environment.
func (r *PingEnvironmentReconciler) listProducts(ctx context.Context, env *pingonev1alpha1.PingEnvironment) (helmclient.ProductSpecs, productCRLists, error) {
	var (
		specs  helmclient.ProductSpecs
		lists  productCRLists
		logger = log.FromContext(ctx)
	)
	fieldSel := client.MatchingFields{"spec.environmentRef": env.Name}
	ns := client.InNamespace(env.Namespace)

	// warnDuplicates logs when more than one CR of a kind references this environment.
	warnDuplicates := func(kind string, n int, winner string) {
		if n > 1 {
			logger.Info("multiple product CRs reference this environment; using the first by name",
				"kind", kind, "count", n, "using", winner)
		}
	}

	pfList := &pingonev1alpha1.PingFederateList{}
	if err := r.List(ctx, pfList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingFederates: %w", err)
	}
	if len(pfList.Items) > 0 {
		sort.Slice(pfList.Items, func(i, j int) bool { return pfList.Items[i].Name < pfList.Items[j].Name })
		warnDuplicates("PingFederate", len(pfList.Items), pfList.Items[0].Name)
		lists.PingFederate = pfList.Items
		specs.PingFederate = &pfList.Items[0].Spec
	}

	pdList := &pingonev1alpha1.PingDirectoryList{}
	if err := r.List(ctx, pdList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingDirectories: %w", err)
	}
	if len(pdList.Items) > 0 {
		sort.Slice(pdList.Items, func(i, j int) bool { return pdList.Items[i].Name < pdList.Items[j].Name })
		warnDuplicates("PingDirectory", len(pdList.Items), pdList.Items[0].Name)
		lists.PingDirectory = pdList.Items
		specs.PingDirectory = &pdList.Items[0].Spec
	}

	paList := &pingonev1alpha1.PingAccessList{}
	if err := r.List(ctx, paList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAccesses: %w", err)
	}
	if len(paList.Items) > 0 {
		sort.Slice(paList.Items, func(i, j int) bool { return paList.Items[i].Name < paList.Items[j].Name })
		warnDuplicates("PingAccess", len(paList.Items), paList.Items[0].Name)
		lists.PingAccess = paList.Items
		specs.PingAccess = &paList.Items[0].Spec
	}

	pazList := &pingonev1alpha1.PingAuthorizeList{}
	if err := r.List(ctx, pazList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAuthorizes: %w", err)
	}
	if len(pazList.Items) > 0 {
		sort.Slice(pazList.Items, func(i, j int) bool { return pazList.Items[i].Name < pazList.Items[j].Name })
		warnDuplicates("PingAuthorize", len(pazList.Items), pazList.Items[0].Name)
		lists.PingAuthorize = pazList.Items
		specs.PingAuthorize = &pazList.Items[0].Spec
	}

	papList := &pingonev1alpha1.PingAuthorizePAPList{}
	if err := r.List(ctx, papList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingAuthorizePAPs: %w", err)
	}
	if len(papList.Items) > 0 {
		sort.Slice(papList.Items, func(i, j int) bool { return papList.Items[i].Name < papList.Items[j].Name })
		warnDuplicates("PingAuthorizePAP", len(papList.Items), papList.Items[0].Name)
		lists.PingAuthorizePAP = papList.Items
		specs.PingAuthorizePAP = &papList.Items[0].Spec
	}

	pdsList := &pingonev1alpha1.PingDataSyncList{}
	if err := r.List(ctx, pdsList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingDataSyncs: %w", err)
	}
	if len(pdsList.Items) > 0 {
		sort.Slice(pdsList.Items, func(i, j int) bool { return pdsList.Items[i].Name < pdsList.Items[j].Name })
		warnDuplicates("PingDataSync", len(pdsList.Items), pdsList.Items[0].Name)
		lists.PingDataSync = pdsList.Items
		specs.PingDataSync = &pdsList.Items[0].Spec
	}

	pdpList := &pingonev1alpha1.PingDirectoryProxyList{}
	if err := r.List(ctx, pdpList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingDirectoryProxies: %w", err)
	}
	if len(pdpList.Items) > 0 {
		sort.Slice(pdpList.Items, func(i, j int) bool { return pdpList.Items[i].Name < pdpList.Items[j].Name })
		warnDuplicates("PingDirectoryProxy", len(pdpList.Items), pdpList.Items[0].Name)
		lists.PingDirectoryProxy = pdpList.Items
		specs.PingDirectoryProxy = &pdpList.Items[0].Spec
	}

	pdcList := &pingonev1alpha1.PingDataConsoleList{}
	if err := r.List(ctx, pdcList, ns, fieldSel); err != nil {
		return specs, lists, fmt.Errorf("list PingDataConsoles: %w", err)
	}
	if len(pdcList.Items) > 0 {
		sort.Slice(pdcList.Items, func(i, j int) bool { return pdcList.Items[i].Name < pdcList.Items[j].Name })
		warnDuplicates("PingDataConsole", len(pdcList.Items), pdcList.Items[0].Name)
		lists.PingDataConsole = pdcList.Items
		specs.PingDataConsole = &pdcList.Items[0].Spec
	}

	return specs, lists, nil
}

func (r *PingEnvironmentReconciler) handleDeletion(ctx context.Context, env *pingonev1alpha1.PingEnvironment) (ctrl.Result, error) {
	targetNS := env.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = env.Namespace
	}
	cfg, err := r.helmConfigFor(targetNS)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("helm client: %w", err)
	}

	releaseName := fmt.Sprintf("%s-ping", env.Spec.TenantID)
	// A failed existence check returns the error so the reconcile retries;
	// removing the finalizer on a transient failure would orphan the release.
	exists, err := helmclient.ReleaseExists(cfg, releaseName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if exists {
		if _, err := action.NewUninstall(cfg).Run(releaseName); err != nil {
			return ctrl.Result{}, fmt.Errorf("uninstall %s: %w", releaseName, err)
		}
	}

	controllerutil.RemoveFinalizer(env, finalizerName)
	return ctrl.Result{}, r.Update(ctx, env)
}

// deployPingDevops renders the values and installs or upgrades the release,
// returning the Helm action taken (installed / upgraded / unchanged).
func (r *PingEnvironmentReconciler) deployPingDevops(cfg *action.Configuration, env *pingonev1alpha1.PingEnvironment, releaseName, namespace string, products helmclient.ProductSpecs) (string, error) {
	values, err := helmclient.BuildPingValues(env.Spec, products)
	if err != nil {
		return "", fmt.Errorf("build ping values: %w", err)
	}

	ch, err := r.loadChart()
	if err != nil {
		return "", err
	}

	return helmclient.InstallOrUpgrade(cfg, releaseName, namespace, ch, values, true)
}

// loadChart returns the cached chart, downloading and parsing it on first call.
// The chart version is pinned so the cached value never becomes stale.
func (r *PingEnvironmentReconciler) loadChart() (*helmChartPkg.Chart, error) {
	chartPath, err := helmclient.DownloadChart(helmRepoURL, helmChartName, helmChartVer, helmCacheDir)
	if err != nil {
		return nil, fmt.Errorf("download %s chart: %w", helmChartName, err)
	}
	if r.cachedChart != nil && r.cachedChart.path == chartPath {
		return r.cachedChart.chart, nil
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load %s chart: %w", helmChartName, err)
	}
	r.cachedChart = &helmChart{path: chartPath, chart: ch}
	return ch, nil
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

func (r *PingEnvironmentReconciler) setFailed(ctx context.Context, env *pingonev1alpha1.PingEnvironment, releaseName string, lists productCRLists, reason, msg string) (ctrl.Result, error) {
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
	// The release (if any) is still deployed after a failed upgrade, so the
	// product statuses keep pointing at it rather than being wiped.
	r.updateProductStatuses(ctx, releaseName, "Failed", lists)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// updateProductStatuses mirrors the environment's phase and release name onto every
// product CR. Patch failures are logged rather than returned: product status is
// informational, and the next reconcile re-patches it anyway.
func (r *PingEnvironmentReconciler) updateProductStatuses(ctx context.Context, releaseName, phase string, lists productCRLists) {
	logger := log.FromContext(ctx)
	patchStatus := func(obj client.Object, kind string, setStatus func()) {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		setStatus()
		if err := r.Status().Patch(ctx, obj, patch); err != nil {
			logger.Error(err, "failed to patch product status", "kind", kind, "name", obj.GetName())
		}
	}
	for i := range lists.PingFederate {
		pf := &lists.PingFederate[i]
		patchStatus(pf, "PingFederate", func() { pf.Status.Phase = phase; pf.Status.Release = releaseName })
	}
	for i := range lists.PingDirectory {
		pd := &lists.PingDirectory[i]
		patchStatus(pd, "PingDirectory", func() { pd.Status.Phase = phase; pd.Status.Release = releaseName })
	}
	for i := range lists.PingAccess {
		pa := &lists.PingAccess[i]
		patchStatus(pa, "PingAccess", func() { pa.Status.Phase = phase; pa.Status.Release = releaseName })
	}
	for i := range lists.PingAuthorize {
		paz := &lists.PingAuthorize[i]
		patchStatus(paz, "PingAuthorize", func() { paz.Status.Phase = phase; paz.Status.Release = releaseName })
	}
	for i := range lists.PingAuthorizePAP {
		pap := &lists.PingAuthorizePAP[i]
		patchStatus(pap, "PingAuthorizePAP", func() { pap.Status.Phase = phase; pap.Status.Release = releaseName })
	}
	for i := range lists.PingDataSync {
		pds := &lists.PingDataSync[i]
		patchStatus(pds, "PingDataSync", func() { pds.Status.Phase = phase; pds.Status.Release = releaseName })
	}
	for i := range lists.PingDirectoryProxy {
		pdp := &lists.PingDirectoryProxy[i]
		patchStatus(pdp, "PingDirectoryProxy", func() { pdp.Status.Phase = phase; pdp.Status.Release = releaseName })
	}
	for i := range lists.PingDataConsole {
		pdc := &lists.PingDataConsole[i]
		patchStatus(pdc, "PingDataConsole", func() { pdc.Status.Phase = phase; pdc.Status.Release = releaseName })
	}
}
