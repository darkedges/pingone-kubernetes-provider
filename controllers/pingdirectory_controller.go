package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingDirectoryReconciler reconciles PingDirectory objects.
type PingDirectoryReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingdirectories/finalizers,verbs=update

func (r *PingDirectoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingDirectory{}).
		Complete(r)
}

func (r *PingDirectoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pd := &pingonev1alpha1.PingDirectory{}
	if err := r.Get(ctx, req.NamespacedName, pd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pd.DeletionTimestamp.IsZero() {
		if containsFinalizer(pd.Finalizers, finalizerName) {
			pd.Finalizers = removeFinalizer(pd.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pd)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pd.Finalizers, finalizerName) {
		pd.Finalizers = append(pd.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pd)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pd.Spec.EnvironmentRef, Namespace: pd.Namespace}, env); err != nil {
		patch := client.MergeFrom(pd.DeepCopy())
		pd.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pd, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
