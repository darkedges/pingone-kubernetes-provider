package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingAuthorizePAPReconciler reconciles PingAuthorizePAP objects.
type PingAuthorizePAPReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizepaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizepaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingauthorizepaps/finalizers,verbs=update

func (r *PingAuthorizePAPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingAuthorizePAP{}).
		Complete(r)
}

func (r *PingAuthorizePAPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pap := &pingonev1alpha1.PingAuthorizePAP{}
	if err := r.Get(ctx, req.NamespacedName, pap); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pap.DeletionTimestamp.IsZero() {
		if containsFinalizer(pap.Finalizers, finalizerName) {
			pap.Finalizers = removeFinalizer(pap.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pap)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pap.Finalizers, finalizerName) {
		pap.Finalizers = append(pap.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pap)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pap.Spec.EnvironmentRef, Namespace: pap.Namespace}, env); err != nil {
		patch := client.MergeFrom(pap.DeepCopy())
		pap.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pap, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
