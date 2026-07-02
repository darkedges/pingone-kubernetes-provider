package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingDirectoryProxyReconciler reconciles PingDirectoryProxy objects.
type PingDirectoryProxyReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectoryproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectoryproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectoryproxies/finalizers,verbs=update

func (r *PingDirectoryProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingDirectoryProxy{}).
		Complete(r)
}

func (r *PingDirectoryProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pdp := &pingonev1alpha1.PingDirectoryProxy{}
	if err := r.Get(ctx, req.NamespacedName, pdp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pdp.DeletionTimestamp.IsZero() {
		if containsFinalizer(pdp.Finalizers, finalizerName) {
			pdp.Finalizers = removeFinalizer(pdp.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pdp)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pdp.Finalizers, finalizerName) {
		pdp.Finalizers = append(pdp.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pdp)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pdp.Spec.EnvironmentRef, Namespace: pdp.Namespace}, env); err != nil {
		patch := client.MergeFrom(pdp.DeepCopy())
		pdp.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pdp, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
