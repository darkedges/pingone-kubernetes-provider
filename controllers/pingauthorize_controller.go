package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingAuthorizeReconciler reconciles PingAuthorize objects.
type PingAuthorizeReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizes/finalizers,verbs=update

func (r *PingAuthorizeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingAuthorize{}).
		Complete(r)
}

func (r *PingAuthorizeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	paz := &pingonev1alpha1.PingAuthorize{}
	if err := r.Get(ctx, req.NamespacedName, paz); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !paz.DeletionTimestamp.IsZero() {
		if containsFinalizer(paz.Finalizers, finalizerName) {
			paz.Finalizers = removeFinalizer(paz.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, paz)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(paz.Finalizers, finalizerName) {
		paz.Finalizers = append(paz.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, paz)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: paz.Spec.EnvironmentRef, Namespace: paz.Namespace}, env); err != nil {
		patch := client.MergeFrom(paz.DeepCopy())
		paz.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, paz, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
