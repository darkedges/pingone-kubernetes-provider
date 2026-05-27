package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingAccessReconciler reconciles PingAccess objects.
type PingAccessReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingaccesses/finalizers,verbs=update

func (r *PingAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingAccess{}).
		Complete(r)
}

func (r *PingAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pa := &pingonev1alpha1.PingAccess{}
	if err := r.Get(ctx, req.NamespacedName, pa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pa.DeletionTimestamp.IsZero() {
		if containsFinalizer(pa.Finalizers, finalizerName) {
			pa.Finalizers = removeFinalizer(pa.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pa)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pa.Finalizers, finalizerName) {
		pa.Finalizers = append(pa.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pa)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pa.Spec.EnvironmentRef, Namespace: pa.Namespace}, env); err != nil {
		patch := client.MergeFrom(pa.DeepCopy())
		pa.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pa, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
