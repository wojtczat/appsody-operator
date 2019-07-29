package appsodyapplication

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsodyv1alpha1 "github.com/appsody-operator/pkg/apis/appsody/v1alpha1"
	appsodyutils "github.com/appsody-operator/pkg/utils"
	routev1 "github.com/openshift/api/route/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_appsodyapplication")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new AppsodyApplication Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAppsodyApplication{ReconcilerBase: appsodyutils.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme())}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("appsodyapplication-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AppsodyApplication
	err = c.Watch(&source.Kind{Type: &appsodyv1alpha1.AppsodyApplication{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner AppsodyApplication
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsodyv1alpha1.AppsodyApplication{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAppsodyApplication implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAppsodyApplication{}

// ReconcileAppsodyApplication reconciles a AppsodyApplication object
type ReconcileAppsodyApplication struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	appsodyutils.ReconcilerBase
}

// Reconcile reads that state of the cluster for a AppsodyApplication object and makes changes based on the state read
// and what is in the AppsodyApplication.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAppsodyApplication) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling AppsodyApplication")

	// Fetch the AppsodyApplication instance
	instance := &appsodyv1alpha1.AppsodyApplication{}
	err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	defaultMeta := metav1.ObjectMeta{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	if instance.Spec.ServiceAccountName == "" {
		serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(serviceAccount, instance, func() error {
			appsodyutils.CustomizeServiceAccount(serviceAccount, instance)
			return nil
		})
		if err != nil {
			reqLogger.Error(err, "Failed to reconcile ServiceAccount")
		}
	} else {
		serviceAccount := &corev1.ServiceAccount{ObjectMeta: defaultMeta}
		err = r.DeleteResource(serviceAccount)
		if err != nil {
			reqLogger.Error(err, "Failed to delete ServiceAccount")
		}
	}

	svc := &corev1.Service{ObjectMeta: defaultMeta}
	err = r.CreateOrUpdate(svc, instance, func() error {
		appsodyutils.CustomizeService(svc, instance)
		return nil
	})
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile Service")
	}

	if instance.Spec.Storage != nil {

		// Delete Deployment if exists
		deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
		err = r.DeleteResource(deploy)

		if err == nil {
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
			err = r.CreateOrUpdate(svc, instance, func() error {
				appsodyutils.CustomizeService(svc, instance)
				svc.Spec.ClusterIP = corev1.ClusterIPNone
				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile headless Service")
			}

			statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(statefulSet, instance, func() error {
				statefulSet.Spec.Replicas = instance.Spec.Replicas
				statefulSet.Spec.ServiceName = instance.Name + "-headless"
				statefulSet.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": instance.Name,
					},
				}
				appsodyutils.CustomizePersistence(statefulSet, instance)
				appsodyutils.CustomizePodSpec(&statefulSet.Spec.Template, instance)
				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile StatefulSet")
			}
		}
	} else {
		// Delete StatefulSet if exists
		statefulSet := &appsv1.StatefulSet{ObjectMeta: defaultMeta}
		err = r.DeleteResource(statefulSet)

		// Delete StatefulSet if exists
		headlesssvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: instance.Name + "-headless", Namespace: instance.Namespace}}
		err = r.DeleteResource(headlesssvc)

		if err == nil {
			deploy := &appsv1.Deployment{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(deploy, instance, func() error {
				deploy.Spec.Replicas = instance.Spec.Replicas
				deploy.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": instance.Name,
					},
				}
				appsodyutils.CustomizePodSpec(&deploy.Spec.Template, instance)
				return nil
			})
			if err != nil {
				reqLogger.Error(err, "Failed to reconcile StatefulSet")
			}
		}
	}

	if instance.Spec.Autoscaling != nil {
		if instance.Spec.Autoscaling.MaxReplicas == nil {
			reqLogger.Error(nil, "Required field Autoscaling.MaxReplicas is not specified")
		} else {
			hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
			err = r.CreateOrUpdate(hpa, instance, func() error {
				appsodyutils.CustomizeHPA(hpa, instance)
				return nil
			})
		}

		if err != nil {
			reqLogger.Error(err, "Failed to reconcile HorizontalPodAutoscaler")
		}
	} else {
		hpa := &autoscalingv1.HorizontalPodAutoscaler{ObjectMeta: defaultMeta}
		err = r.DeleteResource(hpa)
		if err != nil {
			reqLogger.Error(err, "Failed to delete HorizontalPodAutoscaler")
		}
	}

	if instance.Spec.Expose {
		route := &routev1.Route{ObjectMeta: defaultMeta}
		err = r.CreateOrUpdate(route, instance, func() error {
			appsodyutils.CustomizeRoute(route, instance)
			return nil
		})
		if err != nil {
			log.Error(err, "Failed to create Route")
		}

	} else {
		route := &routev1.Route{ObjectMeta: defaultMeta}
		err = r.DeleteResource(route)
		if err != nil {
			log.Error(err, "Failed to delete route")
		}
	}

	return reconcile.Result{}, nil
}
