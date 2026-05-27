package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingFederateReconciler reconciles PingFederate objects.
type PingFederateReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingfederates/finalizers,verbs=update

func (r *PingFederateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingFederate{}).
		Complete(r)
}

func (r *PingFederateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pf := &pingonev1alpha1.PingFederate{}
	if err := r.Get(ctx, req.NamespacedName, pf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pf.DeletionTimestamp.IsZero() {
		if containsFinalizer(pf.Finalizers, finalizerName) {
			pf.Finalizers = removeFinalizer(pf.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pf)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pf.Finalizers, finalizerName) {
		pf.Finalizers = append(pf.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pf)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pf.Spec.EnvironmentRef, Namespace: pf.Namespace}, env); err != nil {
		patch := client.MergeFrom(pf.DeepCopy())
		pf.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pf, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
