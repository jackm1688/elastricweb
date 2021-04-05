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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elasticwebv1 "elasticweb/api/v1"
)

// ElasticwebReconciler reconciles a Elasticweb object
type ElasticwebReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=elasticweb.com.mfz,resources=elasticwebs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticweb.com.mfz,resources=elasticwebs/status,verbs=get;update;patch

func (r *ElasticwebReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("elasticweb", req.NamespacedName)

	// your logic here
	log.Info("1. Start reconcile logic")

	instance := &elasticwebv1.Elasticweb{}

	//pass client query
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("2.1. instance not  found,maybe removed")
			return ctrl.Result{}, err
		}
		log.Error(err, "2.2. error")
		//return error msg to last
		return ctrl.Result{}, err
	}
	log.Info("3. instance:", "deployment", instance.String())

	//query deployment
	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, req.NamespacedName, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("4. deployment not exits")

			if *(instance.Spec.TotalPodQPS) < 1 {
				log.Info("5.1. not need deploy deployment")
				return ctrl.Result{}, nil
			}
			//first create service
			if err = createServiceIfNotExists(ctx, r, instance, req); err != nil {
				log.Error(err, "5.2. create service failed")
				return ctrl.Result{}, err
			}

			//right now create deployment
			if err = createDeployment(ctx, r, instance); err != nil {
				log.Error(err, "5.3. create deployment failed")
				return ctrl.Result{}, err
			}

			//if create success that update
			if err = updateStatus(ctx, r, instance); err != nil {
				log.Error(err, "5.4. update deployment failed")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "6. create deployment and svc failed")
			return ctrl.Result{}, err
		}
	}
	expectReplicas := getExpectReplicas(instance)

	realReplicas := *deployment.Spec.Replicas

	log.Info("7.", " expectReplicas: ", expectReplicas, "realReplicas: ", realReplicas)

	if expectReplicas == realReplicas {
		log.Info("8. return now")
		return ctrl.Result{}, nil
	}

	//if not eq that update
	*(deployment.Spec.Replicas) = expectReplicas
	if err = r.Update(ctx, deployment); err != nil {
		log.Error(err, "9. update deployment replicas error")
		return ctrl.Result{}, err
	}

	log.Info("10. update deployment replicas success")

	//if update deployment success that update status
	if err = updateStatus(ctx, r, instance); err != nil {
		log.Error(err, "11. update status error")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ElasticwebReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticwebv1.Elasticweb{}).
		Complete(r)
}
