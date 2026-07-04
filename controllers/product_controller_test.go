package controllers

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pingonev1alpha1 "github.com/darkedges/pingone-operator/api/v1alpha1"
)

func TestProductReconciler_PendingWhenEnvironmentMissing(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	pf := &pingonev1alpha1.PingFederate{
		ObjectMeta: metav1.ObjectMeta{Name: "pf-pending", Namespace: "default"},
		Spec:       pingonev1alpha1.PingFederateSpec{EnvironmentRef: "no-such-env"},
	}
	if err := k8sClient.Create(ctx, pf); err != nil {
		t.Fatalf("create PingFederate: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pf) })

	waitFor(t, 10*time.Second, "status.phase to become Pending", func() bool {
		got := &pingonev1alpha1.PingFederate{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pf-pending", Namespace: "default"}, got); err != nil {
			return false
		}
		return got.Status.Phase == "Pending"
	})
}

func TestProductReconciler_DoesNotAddFinalizer(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	pd := &pingonev1alpha1.PingDirectory{
		ObjectMeta: metav1.ObjectMeta{Name: "pd-nofinalizer", Namespace: "default"},
		Spec:       pingonev1alpha1.PingDirectorySpec{EnvironmentRef: "no-such-env"},
	}
	if err := k8sClient.Create(ctx, pd); err != nil {
		t.Fatalf("create PingDirectory: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pd) })

	// Give the controller time to reconcile (it will patch status), then
	// verify it never added the legacy cleanup finalizer.
	waitFor(t, 10*time.Second, "controller to reconcile (Pending phase)", func() bool {
		got := &pingonev1alpha1.PingDirectory{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pd-nofinalizer", Namespace: "default"}, got); err != nil {
			return false
		}
		return got.Status.Phase == "Pending"
	})
	holdsFor(t, time.Second, "no finalizer on product CR", func() bool {
		got := &pingonev1alpha1.PingDirectory{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pd-nofinalizer", Namespace: "default"}, got); err != nil {
			return true // deleted by cleanup racing; nothing to assert
		}
		for _, f := range got.Finalizers {
			if f == finalizerName {
				return false
			}
		}
		return true
	})
}

func TestProductReconciler_RemovesLegacyFinalizerOnDelete(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	pa := &pingonev1alpha1.PingAccess{
		ObjectMeta: metav1.ObjectMeta{Name: "pa-legacy", Namespace: "default"},
		Spec:       pingonev1alpha1.PingAccessSpec{EnvironmentRef: "no-such-env"},
	}
	if err := k8sClient.Create(ctx, pa); err != nil {
		t.Fatalf("create PingAccess: %v", err)
	}

	// Simulate a CR created by an operator version that added the finalizer.
	waitFor(t, 5*time.Second, "legacy finalizer to be added", func() bool {
		got := &pingonev1alpha1.PingAccess{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pa-legacy", Namespace: "default"}, got); err != nil {
			return false
		}
		got.Finalizers = append(got.Finalizers, finalizerName)
		return k8sClient.Update(ctx, got) == nil // retried on conflict by polling
	})

	if err := k8sClient.Delete(ctx, pa); err != nil {
		t.Fatalf("delete PingAccess: %v", err)
	}

	// The controller must strip the legacy finalizer so deletion completes.
	waitFor(t, 10*time.Second, "CR to be fully deleted", func() bool {
		got := &pingonev1alpha1.PingAccess{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "pa-legacy", Namespace: "default"}, got)
		return apierrors.IsNotFound(err)
	})
}

func TestListProducts_DeterministicWinner(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	env := &pingonev1alpha1.PingEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-dup", Namespace: "default"},
		Spec:       pingonev1alpha1.PingEnvironmentSpec{TenantID: "dup", Tier: "development"},
	}
	if err := k8sClient.Create(ctx, env); err != nil {
		t.Fatalf("create PingEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, env) })

	// Created in reverse-alphabetical order: the sort in listProducts, not
	// creation or cache order, must pick the winner.
	for _, pf := range []struct {
		name     string
		replicas int32
	}{{"b-pf", 3}, {"a-pf", 2}} {
		obj := &pingonev1alpha1.PingFederate{
			ObjectMeta: metav1.ObjectMeta{Name: pf.name, Namespace: "default"},
			Spec: pingonev1alpha1.PingFederateSpec{
				EnvironmentRef: "env-dup",
				Replicas:       pf.replicas,
			},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create %s: %v", pf.name, err)
		}
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, obj) })
	}

	waitFor(t, 10*time.Second, "listProducts to see both CRs and pick a-pf", func() bool {
		specs, lists, err := envReconciler.listProducts(ctx, env)
		if err != nil {
			return false
		}
		return len(lists.PingFederate) == 2 &&
			specs.PingFederate != nil &&
			specs.PingFederate.Replicas == 2 // a-pf, first by name
	})
}

func TestListProducts_PingDataConsole(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	env := &pingonev1alpha1.PingEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-pdc", Namespace: "default"},
		Spec:       pingonev1alpha1.PingEnvironmentSpec{TenantID: "pdc", Tier: "development"},
	}
	if err := k8sClient.Create(ctx, env); err != nil {
		t.Fatalf("create PingEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, env) })

	pdc := &pingonev1alpha1.PingDataConsole{
		ObjectMeta: metav1.ObjectMeta{Name: "pdc-console", Namespace: "default"},
		Spec: pingonev1alpha1.PingDataConsoleSpec{
			EnvironmentRef: "env-pdc",
			Config:         pingonev1alpha1.PingDataConsoleConfig{BrandingAppName: "Console"},
		},
	}
	if err := k8sClient.Create(ctx, pdc); err != nil {
		t.Fatalf("create PingDataConsole: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pdc) })

	waitFor(t, 10*time.Second, "listProducts to return the PingDataConsole spec", func() bool {
		specs, lists, err := envReconciler.listProducts(ctx, env)
		if err != nil {
			return false
		}
		return len(lists.PingDataConsole) == 1 &&
			specs.PingDataConsole != nil &&
			specs.PingDataConsole.Config.BrandingAppName == "Console"
	})
}

func TestCRDValidation_RejectsInvalidSpec(t *testing.T) {
	requireEnv(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		env  pingonev1alpha1.PingEnvironmentSpec
	}{
		{"invalid tier", pingonev1alpha1.PingEnvironmentSpec{TenantID: "ok", Tier: "gigantic"}},
		{"uppercase tenantId", pingonev1alpha1.PingEnvironmentSpec{TenantID: "NotDNS", Tier: "development"}},
		{"empty tenantId", pingonev1alpha1.PingEnvironmentSpec{TenantID: "", Tier: "development"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := &pingonev1alpha1.PingEnvironment{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "invalid-", Namespace: "default"},
				Spec:       tc.env,
			}
			if err := k8sClient.Create(ctx, obj); err == nil {
				_ = k8sClient.Delete(ctx, obj)
				t.Fatalf("expected CRD validation to reject %s", tc.name)
			} else if !apierrors.IsInvalid(err) {
				t.Fatalf("expected Invalid error, got: %v", err)
			}
		})
	}
}

func TestEnqueueFromEnvironmentRef(t *testing.T) {
	pf := &pingonev1alpha1.PingFederate{
		ObjectMeta: metav1.ObjectMeta{Name: "pf", Namespace: "ns1"},
		Spec:       pingonev1alpha1.PingFederateSpec{EnvironmentRef: "my-env"},
	}
	reqs := enqueueFromEnvironmentRef(context.Background(), pf)
	if len(reqs) != 1 || reqs[0].Name != "my-env" || reqs[0].Namespace != "ns1" {
		t.Errorf("unexpected requests for product CR: %v", reqs)
	}

	empty := &pingonev1alpha1.PingDirectory{ObjectMeta: metav1.ObjectMeta{Name: "pd", Namespace: "ns1"}}
	if reqs := enqueueFromEnvironmentRef(context.Background(), empty); reqs != nil {
		t.Errorf("expected nil for empty environmentRef, got %v", reqs)
	}

	var notProduct client.Object = &corev1.Pod{}
	if reqs := enqueueFromEnvironmentRef(context.Background(), notProduct); reqs != nil {
		t.Errorf("expected nil for non-product object, got %v", reqs)
	}
}
