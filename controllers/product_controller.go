package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// ProductObject is implemented by every product CR (PingFederate, PingDirectory,
// PingAccess, PingAuthorize, PingAuthorizePAP, PingDataSync, PingDirectoryProxy,
// PingDataConsole). It gives the shared reconciler and the environmentRef
// index/enqueue plumbing a uniform view of the per-product types.
type ProductObject interface {
	client.Object
	GetEnvironmentRef() string
	SetPhase(phase string)
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates;pingdirectories;pingaccesses;pingauthorizes;pingauthorizepaps;pingdatasyncs;pingdirectoryproxies;pingdataconsoles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates/status;pingdirectories/status;pingaccesses/status;pingauthorizes/status;pingauthorizepaps/status;pingdatasyncs/status;pingdirectoryproxies/status;pingdataconsoles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates/finalizers;pingdirectories/finalizers;pingaccesses/finalizers;pingauthorizes/finalizers;pingauthorizepaps/finalizers;pingdatasyncs/finalizers;pingdirectoryproxies/finalizers;pingdataconsoles/finalizers,verbs=update

// ProductReconciler reconciles any product CR. The heavy lifting (rendering the
// product into the Helm release) happens in the PingEnvironment reconciler; this
// controller only maintains the product's own lifecycle signal: Pending while the
// referenced PingEnvironment does not exist.
type ProductReconciler struct {
	client.Client
	// NewObject returns an empty instance of the product type this
	// reconciler instance manages.
	NewObject func() ProductObject
	// Kind is the product kind name, used for the controller name and logs.
	Kind string
}

func (r *ProductReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.NewObject()).
		Complete(r)
}

func (r *ProductReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.NewObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		// Earlier operator versions added a cleanup finalizer to product CRs even
		// though there is no per-product cleanup action (the PingEnvironment
		// reconciler re-renders the release when a product CR disappears).
		// New CRs no longer get the finalizer; this removal only unblocks
		// deletion of CRs created by those earlier versions.
		if controllerutil.RemoveFinalizer(obj, finalizerName) {
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	env := &pingonev1alpha1.PingEnvironment{}
	err := r.Get(ctx, types.NamespacedName{Name: obj.GetEnvironmentRef(), Namespace: obj.GetNamespace()}, env)
	if err != nil {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		obj.SetPhase("Pending")
		_ = r.Status().Patch(ctx, obj, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
