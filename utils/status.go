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

func (r *ReconcilerBase) CheckComponentStatus(recctx *ReconcileContext, ba common.BaseComponent) (reconcile.Result, error) {

	// Create status condition if Available condition type does not exit
	s := ba.GetStatus()
	sc := s.GetCondition(common.StatusConditionTypeResourceAvailable)
	if sc == nil {
		sc = s.NewCondition()
		sc.SetType(common.StatusConditionTypeResourceAvailable)
		sc.SetStatus(corev1.ConditionUnknown)
		sc.SetMessage("")
		sc.SetReason("")
	}

	status, message, reason := corev1.ConditionFalse, "", ""
	// Check for Deployment, StatefulSet or Knative status
	if ba.GetCreateKnativeService() == nil || *ba.GetCreateKnativeService() != true {
		if ba.GetStatefulSet() == nil {
			status, message, reason = r.isDeploymentAvailable(recctx, ba)
		} else {
			status, message, reason = r.isStatefulSetAvailable(recctx, ba)
		}
	} else {
		status, message = r.isKnativeAvailable(recctx, ba)
	}

	// Check if status changed
	if sc.GetMessage() != message || sc.GetStatus() != status {
		// Set condition and update status
		transitionTime := metav1.Now()
		sc.SetLastTransitionTime(&transitionTime)
		sc.SetStatus(status)
		sc.SetMessage(message)
		sc.SetReason(reason)
		s.SetCondition(sc)

		err := r.UpdateStatus(ba.(client.Object))
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

func (r *ReconcilerBase) isDeploymentAvailable(recctx *ReconcileContext, ba common.BaseComponent) (corev1.ConditionStatus, string, string) {
	deployment := &appsv1.Deployment{}
	msg, reason := "Deployment is not available.", "MinimumReplicasUnavailable"

	// Check if deployment exists
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, deployment); err != nil {
		reason = "NotCreated"
		return corev1.ConditionFalse, msg, reason
	}

	ds := deployment.Status
	expectedReplicas := ba.GetReplicas()

	// Deployment replicas check
	if expectedReplicas != nil {
		msg = "Deployment replicas ready: " + strconv.Itoa(int(ds.ReadyReplicas)) + "/" + strconv.Itoa(int(*expectedReplicas))

		if ds.Replicas == *expectedReplicas && ds.ReadyReplicas == *expectedReplicas && ds.UpdatedReplicas == *expectedReplicas {
			reason := "MinimumReplicasAvailable"
			return corev1.ConditionTrue, msg, reason
		}
	}

	return corev1.ConditionFalse, msg, reason
}

func (r *ReconcilerBase) isStatefulSetAvailable(recctx *ReconcileContext, ba common.BaseComponent) (corev1.ConditionStatus, string, string) {
	statefulSet := &appsv1.StatefulSet{}
	msg, reason := "StatefulSet is not available.", "MinimumReplicasUnavailable"

	// Check if statefulset exists
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, statefulSet); err != nil {
		reason = "NotCreated"
		return corev1.ConditionFalse, msg, reason
	}

	ss := statefulSet.Status
	expectedReplicas := ba.GetReplicas()

	// StatefulSet replicas check
	if expectedReplicas != nil {
		msg = "StatefulSet replicas ready: " + strconv.Itoa(int(ss.ReadyReplicas)) + "/" + strconv.Itoa(int(*expectedReplicas))

		if ss.Replicas == *expectedReplicas && ss.ReadyReplicas == *expectedReplicas && ss.UpdatedReplicas == *expectedReplicas {
			reason := "MinimumReplicasAvailable"
			return corev1.ConditionTrue, msg, reason
		}
	}
	return corev1.ConditionFalse, msg, reason
}

func (r *ReconcilerBase) isKnativeAvailable(recctx *ReconcileContext, ba common.BaseComponent) (corev1.ConditionStatus, string) {
	knative := &servingv1.Service{}

	// Check if knative service exists
	if err := r.GetClient().Get(*recctx.Ctx, recctx.Req.NamespacedName, knative); err != nil {
		return corev1.ConditionFalse, "Knative service is not available."
	}
	return corev1.ConditionTrue, "Knative service is available."
}
