package controllers

import (
	"context"
	elasticwebv1 "elasticweb/api/v1"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	APP_NAME       = "elastic-app"
	CONTAINER_PORT = 8080
	CPU_REQUEST    = "100m"
	CPU_LIMIT      = "1OOm"
	MEM_REQUEST    = "512Mi"
	MEM_LIMIT      = "512Mi"
)

//根定单个QPS和总QPS计算pod的数量
func getExpectReplicas(elasticweb *elasticwebv1.Elasticweb) int32 {

	//single pod of QPS
	singlePodQPS := *(elasticweb.Spec.SinglePodQPS)
	//期望的总QPS
	totalQPS := *(elasticweb.Spec.TotalPodQPS)
	//Replicas就是要创建复本数
	replicas := totalQPS / singlePodQPS

	if totalQPS%singlePodQPS > 0 {
		replicas++
	}
	return replicas
}

//create service
func createServiceIfNotExists(ctx context.Context, r *ElasticwebReconciler, elasticweb *elasticwebv1.Elasticweb, req ctrl.Request) error {

	log := r.Log.WithValues("fun", "createService")

	service := &corev1.Service{}

	err := r.Get(ctx, req.NamespacedName, service)

	if err == nil {
		log.Info("service exsists")
		return nil
	}

	//if err is not NotFound,return
	if !errors.IsNotFound(err) {
		log.Error(err, "query service failed")
		return err
	}

	//instance a struct
	service = &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      elasticweb.Name,
			Namespace: elasticweb.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name: "http",
				Port: 80,
				TargetPort: intstr.IntOrString{
					IntVal: CONTAINER_PORT,
				},
			}},
			Selector: map[string]string{
				"app": APP_NAME,
			},
			Type: "NodePort",
		},
	}
	//build con
	log.Info("set reference")
	if err := controllerutil.SetControllerReference(elasticweb, service, r.Scheme); err != nil {
		log.Error(err, "setControllerReference err")
		return err
	}

	//create service
	log.Info("begin to create service")
	if err := r.Create(ctx, service); err != nil {
		log.Error(err, "create service resource failed")
		return err
	}

	log.Info("create service success")
	return nil
}

//create deployment
func createDeployment(ctx context.Context, r *ElasticwebReconciler, elasticweb *elasticwebv1.Elasticweb) error {
	log := r.Log.WithValues("func", "createDeployment")

	//计算期望的pod数量
	expectReplicas := getExpectReplicas(elasticweb)

	//instance a struct
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      elasticweb.Name,
			Namespace: elasticweb.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &expectReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": APP_NAME,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": APP_NAME,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            APP_NAME,
						Image:           elasticweb.Spec.Image,
						ImagePullPolicy: "IfNotPresent",
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: CONTAINER_PORT,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    resource.MustParse(CPU_REQUEST),
								"memory": resource.MustParse(MEM_REQUEST),
							},
							/*Limits: corev1.ResourceList{
								"cpu":    resource.MustParse(CPU_LIMIT),
								"memory": resource.MustParse(MEM_LIMIT),
							},*/
						},
					}},
				},
			},
		},
	}
	log.Info("set deployment reference")
	if err := r.Create(ctx, deployment); err != nil {
		log.Error(err, "create deployment failed")
		return err
	}

	log.Info("create deployment success")
	return nil
}

func updateStatus(ctx context.Context, r *ElasticwebReconciler, elasticweb *elasticwebv1.Elasticweb) error {
	log := r.Log.WithValues("func", "updateStatus")

	//single pod num of qps
	singlePodQPS := *(elasticweb.Spec.SinglePodQPS)

	//pod num
	replicas := getExpectReplicas(elasticweb)

	//current pod create after,current system real value
	if elasticweb.Status.RealQPS == nil {
		elasticweb.Status.RealQPS = new(int32)
	}

	*(elasticweb.Status.RealQPS) = singlePodQPS * replicas

	log.Info(fmt.Sprintf("singlePodQPS [%d], replicas [%d], realQPS:[%d]", singlePodQPS, replicas, *(elasticweb.Status.RealQPS)))
	if err := r.Update(ctx, elasticweb); err != nil {
		log.Error(err, "update instance error")
		return err
	}
	return nil

}
