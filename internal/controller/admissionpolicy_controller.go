/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"

	"github.com/go-logr/logr"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policiesv1 "github.com/kubewarden/kubewarden-controller/api/policies/v1"
)

// Warning: this controller is deployed by a helm chart which has its own
// templated RBAC rules. The rules are kept in sync between what is generated by
// `make manifests` and the helm chart by hand.
//
// We need access to these resources inside of all the namespaces -> a ClusterRole
// is needed
//+kubebuilder:rbac:groups=policies.kubewarden.io,resources=admissionpolicies,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=policies.kubewarden.io,resources=admissionpolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=policies.kubewarden.io,resources=admissionpolicies/finalizers,verbs=update
//
// Some RBAC rules needed to access some resources used here are defined in the
// policyserver_controller.go file.

// AdmissionPolicyReconciler reconciles an AdmissionPolicy object.
type AdmissionPolicyReconciler struct {
	client.Client
	Log                                        logr.Logger
	Scheme                                     *runtime.Scheme
	DeploymentsNamespace                       string
	FeatureGateAdmissionWebhookMatchConditions bool
	policySubReconciler                        *policySubReconciler
}

// Reconcile reconciles admission policies.
func (r *AdmissionPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var admissionPolicy policiesv1.AdmissionPolicy
	if err := r.Get(ctx, req.NamespacedName, &admissionPolicy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.policySubReconciler.reconcile(ctx, &admissionPolicy)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AdmissionPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.policySubReconciler = &policySubReconciler{
		r.Client,
		r.Log,
		r.DeploymentsNamespace,
		r.FeatureGateAdmissionWebhookMatchConditions,
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&policiesv1.AdmissionPolicy{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findAdmissionPoliciesForPod),
		).
		Watches(
			&admissionregistrationv1.ValidatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.findAdmissionPolicyForWebhookConfiguration),
		).
		Watches(
			&admissionregistrationv1.MutatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.findAdmissionPolicyForWebhookConfiguration),
		).
		Complete(r)
	if err != nil {
		return errors.Join(errors.New("failed enrolling controller with manager"), err)
	}

	return nil
}

func (r *AdmissionPolicyReconciler) findAdmissionPoliciesForPod(ctx context.Context, object client.Object) []reconcile.Request {
	return findPoliciesForPod(ctx, r.Client, object)
}

func (r *AdmissionPolicyReconciler) findAdmissionPolicyForWebhookConfiguration(_ context.Context, webhookConfiguration client.Object) []reconcile.Request {
	return findPolicyForWebhookConfiguration(webhookConfiguration, false, r.Log)
}
