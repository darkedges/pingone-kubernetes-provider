package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

// PingDataSyncReconciler reconciles PingDataSync objects.
type PingDataSyncReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=pingone.io,resources=pingdatasyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pingone.io,resources=pingdatasyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pingone.io,resources=pingdatasyncs/finalizers,verbs=update

func (r *PingDataSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pingonev1alpha1.PingDataSync{}).
		Complete(r)
}

func (r *PingDataSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pds := &pingonev1alpha1.PingDataSync{}
	if err := r.Get(ctx, req.NamespacedName, pds); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pds.DeletionTimestamp.IsZero() {
		if containsFinalizer(pds.Finalizers, finalizerName) {
			pds.Finalizers = removeFinalizer(pds.Finalizers, finalizerName)
			return ctrl.Result{}, r.Update(ctx, pds)
		}
		return ctrl.Result{}, nil
	}

	if !containsFinalizer(pds.Finalizers, finalizerName) {
		pds.Finalizers = append(pds.Finalizers, finalizerName)
		return ctrl.Result{Requeue: true}, r.Update(ctx, pds)
	}

	env := &pingonev1alpha1.PingEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pds.Spec.EnvironmentRef, Namespace: pds.Namespace}, env); err != nil {
		patch := client.MergeFrom(pds.DeepCopy())
		pds.Status.Phase = "Pending"
		_ = r.Status().Patch(ctx, pds, patch)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
