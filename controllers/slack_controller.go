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

package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hookv1alpha1 "github.com/seongjumoon/hook-operator/api/v1alpha1"
)

// SlackReconciler reconciles a Slack object
type SlackReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hook.overconfigured.dev,resources=slacks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hook.overconfigured.dev,resources=slacks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

func (r *SlackReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("slack", req.NamespacedName)

	slack := &hookv1alpha1.Slack{}

	err := r.Get(ctx, req.NamespacedName, slack)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Slack Resource must be deleted or not provisioning")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Slack")
		return ctrl.Result{}, err
	}

	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: slack.Name, Namespace: slack.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentSlack(slack)
		log.Info("New Deployment slack deployed", "Deployment.Name", dep.Name, "Deployment.Namespace", dep.Namespace)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed Create Deployment ", "Deployment.Name", dep.Name, "Deployment.Namespace", dep.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	size := slack.Spec.Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
	}

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(slack.Namespace),
		client.MatchingLabels(labelForSlack(slack.Name)),
	}

	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "Slack.Namespace", slack.Namespace, "Slack.Name", slack.Name)
		return ctrl.Result{}, err
	}

	podNames := getPodNames(podList.Items)

	if !reflect.DeepEqual(podNames, slack.Status.Endpoints) {
		slack.Status.Endpoints = podNames
		err := r.Status().Update(ctx, slack)
		if err != nil {
			log.Error(err, "Failed to update Slack Endpoints")
		}
	}

	return ctrl.Result{}, nil
}

func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

func (r *SlackReconciler) deploymentSlack(s *hookv1alpha1.Slack) *appsv1.Deployment {
	ls := labelForSlack(s.Name)
	replicas := s.Spec.Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "nginx", // for test
							Name:  "Custom-nginx",
						},
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(s, dep, r.Scheme)
	return dep
}

func labelForSlack(name string) map[string]string {
	return map[string]string{"app": "slack", "workspace": name}
}

func (r *SlackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hookv1alpha1.Slack{}).
		Owns(&appsv1.Deployment{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
		}).
		Complete(r)
}
