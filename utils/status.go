/*


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

package utils

import (
	"strconv"
	"time"

	"github.com/application-stacks/runtime-component-operator/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type replicasStatus struct {
	ReadyReplicas   int32
	UpdatedReplicas int32
}

func (r *ReconcilerBase) CheckComponentStatus(ba common.BaseComponent) (reconcile.Result, error) {
	// Create status condition if Component Ready condition type does not exit
	s := ba.GetStatus()
	sc := s.GetCondition(common.StatusConditionTypeComponentReady)
	if sc == nil {
		sc = s.NewCondition()
		sc.SetType(common.StatusConditionTypeComponentReady)
		sc.SetStatus(corev1.ConditionUnknown)
		sc.SetMessage("")
	}

	status, message, reason := corev1.ConditionFalse, "", ""

	scReconciled := s.GetCondition(common.StatusConditionTypeReconciled)
	scReady := s.GetCondition(common.StatusConditionTypeResourceReady)

	// Check reconciled and resource status
	if scReconciled != nil && scReconciled.GetStatus() == corev1.ConditionTrue && scReady != nil && scReady.GetStatus() == corev1.ConditionTrue {
		status = corev1.ConditionTrue
		message = "RuntimeComponent is reconciled and resources are ready"
		reason = "ComponentReady"
	} else if scReconciled == nil || scReconciled.GetStatus() != corev1.ConditionTrue {
		message = "RuntimeComponent is not reconciled"
		reason = "ComponentNotReconciled"
	} else {
		message = "Resources are not ready"
		reason = "ResourceNotReady"
	}

	return r.updateStatusConditions(ba, s, sc, status, message, reason)
}

func (r *ReconcilerBase) CheckResourceStatus(recctx *ReconcileContext, ba common.BaseComponent, defaultMeta metav1.ObjectMeta) (reconcile.Result, error) {
	// Create status condition if Resource Ready condition type does not exit
	s := ba.GetStatus()
	sc := s.GetCondition(common.StatusConditionTypeResourceReady)
	if sc == nil {
		sc = s.NewCondition()
		sc.SetType(common.StatusConditionTypeResourceReady)
		sc.SetStatus(corev1.ConditionUnknown)
		sc.SetMessage("")
		sc.SetReason("")
	}

	status, message, reason := corev1.ConditionFalse, "", ""
	// Check for Deployment, StatefulSet or Knative status
	if ba.GetCreateKnativeService() == nil || *ba.GetCreateKnativeService() != true {
		if ba.GetStatefulSet() == nil {
			status, message, reason = r.isDeploymentReady(recctx, ba, defaultMeta)
		} else {
			status, message, reason = r.isStatefulSetReady(recctx, ba, defaultMeta)
		}
	} else {
		status, message = r.isKnativeReady(recctx, ba, defaultMeta)
	}

	return r.updateStatusConditions(ba, s, sc, status, message, reason)
}

func (r *ReconcilerBase) updateStatusConditions(ba common.BaseComponent, s common.BaseComponentStatus, sc common.StatusCondition, status corev1.ConditionStatus, message string, reason string) (reconcile.Result, error) {
	// Check if status condition changed
	if sc.GetStatus() != status || sc.GetMessage() != message {
		// Set condition and update status
		transitionTime := metav1.Now()
		sc.SetLastTransitionTime(&transitionTime)
		sc.SetStatus(status)
		sc.SetMessage(message)
		sc.SetReason(reason)
		s.SetCondition(sc)

		err := r.UpdateStatus(ba.(client.Object))
		// Detect errors while updating status
		if err != nil {
			log.Error(err, "Unable to update status")
			return reconcile.Result{
				RequeueAfter: time.Second,
				Requeue:      true,
			}, nil
		}
	}
	return reconcile.Result{RequeueAfter: ReconcileInterval * time.Second}, nil
}

func (r *ReconcilerBase) isDeploymentReady(recctx *ReconcileContext, ba common.BaseComponent, defaultMeta metav1.ObjectMeta) (corev1.ConditionStatus, string, string) {
	// Check if deployment exists
	deployment := &appsv1.Deployment{ObjectMeta: defaultMeta}
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, deployment); err != nil {
		msg, reason := "Deployment is not ready.", "NotCreated"
		return corev1.ConditionFalse, msg, reason
	}

	resourceType := "Deployment"
	ds := deployment.Status
	replicas := replicasStatus{ReadyReplicas: ds.ReadyReplicas, UpdatedReplicas: ds.UpdatedReplicas}

	return r.areReplicasReady(replicas, ba, resourceType)
}

func (r *ReconcilerBase) isStatefulSetReady(recctx *ReconcileContext, ba common.BaseComponent, defaultMeta metav1.ObjectMeta) (corev1.ConditionStatus, string, string) {
	// Check if statefulset exists
	statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, statefulSet); err != nil {
		msg, reason := "StatefulSet is not ready.", "NotCreated"
		return corev1.ConditionFalse, msg, reason
	}

	resourceType := "StatefulSet"
	ss := statefulSet.Status
	rs := replicasStatus{ReadyReplicas: ss.ReadyReplicas, UpdatedReplicas: ss.UpdatedReplicas}

	return r.areReplicasReady(rs, ba, resourceType)
}

func (r *ReconcilerBase) areReplicasReady(rs replicasStatus, ba common.BaseComponent, resourceType string) (corev1.ConditionStatus, string, string) {
	expectedReplicas := ba.GetReplicas()
	msg, reason := resourceType+" is not ready", "MinimumReplicasUnavailable"

	// Replicas check
	if expectedReplicas != nil {
		msg = resourceType + " replicas ready: " + strconv.Itoa(int(rs.ReadyReplicas)) + "/" + strconv.Itoa(int(*expectedReplicas))

		if rs.ReadyReplicas == *expectedReplicas && rs.UpdatedReplicas == *expectedReplicas {
			reason = "MinimumReplicasAvailable"
			return corev1.ConditionTrue, msg, reason
		}
	}
	return corev1.ConditionFalse, msg, reason
}

func (r *ReconcilerBase) isKnativeReady(recctx *ReconcileContext, ba common.BaseComponent, defaultMeta metav1.ObjectMeta) (corev1.ConditionStatus, string) {
	knative := &servingv1.Service{ObjectMeta: defaultMeta}

	// Check if knative service exists
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, knative); err != nil {
		return corev1.ConditionFalse, "Knative service is not ready."
	}
	return corev1.ConditionTrue, "Knative service is ready."
}
