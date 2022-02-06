package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	v1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
)

var commandUsageExample = `
	# scale a deployment to 0 replicas then back up to the original count
	kubectl rescale deployment/nginx

	# scale a statefulset to 0 replicas then back up to the original count
	kubectl rescale statefulset/mysql

	# scale a statefulset to 0 replicas then back up to the original count, and wait for a maximum of 600 seconds to do so
	kubectl rescale statefulset/mysql --max-wait-seconds=600

	# it also supports short names
	kubectl rescale sts/mysql

	# if the kind is not provided, it will first try to find a deployment with the supplied name, and if not found then statefulset
	kubectl rescale nginx

	# a namespace can also be supplied
	kubectl rescale deployment/nginx -n dev
`

var errNoContext = fmt.Errorf("no or invalid context is set, use %q to select a new one", "kubectl config use-context <context>")

// RescaleOptions provides information required to update
// the current context on a user's KUBECONFIG
type RescaleOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams

	userSpecifiedContext   string
	userSpecifiedNamespace string

	rawConfig  api.Config
	restConfig *rest.Config

	targetName string
	targetKind string

	maxWaitSeconds int
}

// NewRescaleOptions provides an instance of RescaleOptions with default values
func NewRescaleOptions(streams genericclioptions.IOStreams) *RescaleOptions {
	return &RescaleOptions{
		configFlags: genericclioptions.NewConfigFlags(true),
		IOStreams:   streams,
	}
}

// NewCmdRescale provides a cobra command wrapping RescaleOptions
func NewCmdRescale(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRescaleOptions(streams)

	cmd := &cobra.Command{
		Use:          "rescale [name of deployment/statefulset] [flags]",
		Short:        "Scale a deployment or statefulset to 0 then back up",
		Example:      commandUsageExample,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.PersistentFlags().IntP("max-wait-seconds", "w", 300, "max number of seconds to wait for the scaled objects to reach desired number of replicas [default: 300]")

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for updating the current context
func (o *RescaleOptions) Complete(cmd *cobra.Command, args []string) error {
	var err error

	o.rawConfig, err = o.configFlags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return err
	}

	o.restConfig, err = o.configFlags.ToRESTConfig()
	if err != nil {
		panic(err.Error())
	}

	o.userSpecifiedContext, err = cmd.Flags().GetString("context")
	if err != nil {
		return err
	}
	if len(o.userSpecifiedContext) > 0 {
		if _, found := o.rawConfig.Contexts[o.userSpecifiedContext]; !found {
			return errNoContext
		}
	} else if len(o.rawConfig.CurrentContext) == 0 {
		return errNoContext
	}

	o.maxWaitSeconds, err = cmd.Flags().GetInt("max-wait-seconds")
	if err != nil {
		return err
	}
	if o.maxWaitSeconds <= 0 {
		return fmt.Errorf("invalid max number of waiting seconds provided")
	}

	if len(args) != 1 {
		return fmt.Errorf("either a deployment or a statefulset must be provided")
	}
	if strings.HasPrefix(args[0], "deployment/") {
		o.targetName = strings.TrimPrefix(args[0], "deployment/")
		o.targetKind = "deployment"
	} else if strings.HasPrefix(args[0], "deploy/") {
		o.targetName = strings.TrimPrefix(args[0], "deploy/")
		o.targetKind = "deployment"
	} else if strings.HasPrefix(args[0], "statefulset/") {
		o.targetName = strings.TrimPrefix(args[0], "statefulset/")
		o.targetKind = "statefulset"
	} else if strings.HasPrefix(args[0], "sts/") {
		o.targetName = strings.TrimPrefix(args[0], "sts/")
		o.targetKind = "statefulset"
	} else {
		o.targetName = args[0]
		o.targetKind = "unknown"
	}

	o.userSpecifiedNamespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	return nil
}

// Run lists all available namespaces on a user's KUBECONFIG or updates the
// current context based on a provided namespace.
func (o *RescaleOptions) Run() error {
	clientset, err := kubernetes.NewForConfig(o.restConfig)
	if err != nil {
		panic(err.Error())
	}

	var ctx string
	if len(o.userSpecifiedContext) > 0 {
		ctx = o.userSpecifiedContext
	} else {
		ctx = o.rawConfig.CurrentContext
	}

	var namespace string
	if len(o.userSpecifiedNamespace) > 0 {
		namespace = o.userSpecifiedNamespace
	} else {
		namespace = o.rawConfig.Contexts[ctx].Namespace
	}

	if o.targetKind == "unknown" {
		_, err = GetDeployment(clientset, namespace, o.targetName)
		if err != nil {
			if errors.IsNotFound(err) {
				_, err = GetStatefulSet(clientset, namespace, o.targetName)
				if errors.IsNotFound(err) {
					notFoundError := fmt.Errorf("deployment/statefulset %s cannot be found", o.targetName)
					fmt.Println(notFoundError.Error())
					return notFoundError
				} else if err != nil {
					return err
				} else {
					o.targetKind = "statefulset"
				}
			} else {
				return err
			}
		} else {
			o.targetKind = "deployment"
		}
	}

	if o.targetKind == "deployment" {
		err = ScaleDeployment(clientset, namespace, o.targetName, o.maxWaitSeconds)
		if err != nil {
			return err
		}
	} else if o.targetKind == "statefulset" {
		err = ScaleStatefulSet(clientset, namespace, o.targetName, o.maxWaitSeconds)
		if err != nil {
			return err
		}
	} else {
		notFoundError := fmt.Errorf("unknown target kind %s", o.targetKind)
		fmt.Println(notFoundError.Error())
		return notFoundError
	}

	return nil
}

func ScaleDeployment(clientset *kubernetes.Clientset, namespace string, targetName string, maxWaitSeconds int) error {
	deployment, err := GetDeployment(clientset, namespace, targetName)
	if err != nil {
		return err
	}

	var originalReplicas = deployment.Status.Replicas
	fmt.Printf("Deployment %s in %s has %d replicas. Scaling to 0...\n", deployment.Name, namespace, originalReplicas)

	_, err = UpdateDeploymentScale(clientset, namespace, targetName, 0)
	if err != nil {
		panic(err.Error())
	}

	err = WaitForDeploymentReplicas(clientset, namespace, targetName, 0, maxWaitSeconds)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Deployment %s in %s now has 0 replicas. Scaling back to %d...\n", deployment.Name, namespace, originalReplicas)

	_, err = UpdateDeploymentScale(clientset, namespace, targetName, originalReplicas)
	if err != nil {
		panic(err.Error())
	}

	err = WaitForDeploymentReplicas(clientset, namespace, targetName, originalReplicas, 60)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Deployment %s in %s has now been scaled back to %d\n", deployment.Name, namespace, originalReplicas)

	return nil
}

func ScaleStatefulSet(clientset *kubernetes.Clientset, namespace string, targetName string, maxWaitSeconds int) error {
	statefulSet, err := GetStatefulSet(clientset, namespace, targetName)
	if err != nil {
		return err
	}

	var originalReplicas = statefulSet.Status.Replicas
	fmt.Printf("StatefulSet %s in %s has %d replicas. Scaling to 0...\n", statefulSet.Name, namespace, originalReplicas)

	_, err = UpdateStatefulSetScale(clientset, namespace, targetName, 0)
	if err != nil {
		panic(err.Error())
	}

	err = WaitForStatefulSetReplicas(clientset, namespace, targetName, 0, maxWaitSeconds)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("StatefulSet %s in %s now has 0 replicas. Scaling back to %d...\n", statefulSet.Name, namespace, originalReplicas)

	_, err = UpdateStatefulSetScale(clientset, namespace, targetName, originalReplicas)
	if err != nil {
		panic(err.Error())
	}

	err = WaitForStatefulSetReplicas(clientset, namespace, targetName, originalReplicas, 60)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("StatefulSet %s in %s has now been scaled back to %d\n", statefulSet.Name, namespace, originalReplicas)

	return nil
}

func GetDeployment(clientset *kubernetes.Clientset, namespace string, targetName string) (*v1.Deployment, error) {
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, err
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting deployment %s in %s: %v\n", targetName, namespace, statusError.ErrStatus.Message)
		return nil, err
	} else if err != nil {
		panic(err.Error())
	}

	return deployment, err
}

func GetStatefulSet(clientset *kubernetes.Clientset, namespace string, targetName string) (*v1.StatefulSet, error) {
	statefulSet, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, err
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting statefulset %s in %s: %v\n", targetName, namespace, statusError.ErrStatus.Message)
		return nil, err
	} else if err != nil {
		panic(err.Error())
	}

	return statefulSet, err
}

func UpdateDeploymentScale(clientset *kubernetes.Clientset, namespace string, targetName string, replicas int32) (*autoscalingv1.Scale, error) {
	scale, err := clientset.AppsV1().Deployments(namespace).GetScale(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	scale.Spec.Replicas = replicas
	_, err = clientset.AppsV1().Deployments(namespace).UpdateScale(context.TODO(), targetName, scale, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return scale, nil
}

func UpdateStatefulSetScale(clientset *kubernetes.Clientset, namespace string, targetName string, replicas int32) (*autoscalingv1.Scale, error) {
	scale, err := clientset.AppsV1().StatefulSets(namespace).GetScale(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	scale.Spec.Replicas = replicas
	_, err = clientset.AppsV1().StatefulSets(namespace).UpdateScale(context.TODO(), targetName, scale, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return scale, nil
}

func WaitForDeploymentReplicas(clientset *kubernetes.Clientset, namespace string, targetName string, replicas int32, tries int) error {
	for i := 0; i < tries; i++ {
		scale, _ := clientset.AppsV1().Deployments(namespace).GetScale(context.TODO(), targetName, metav1.GetOptions{})
		if scale.Status.Replicas == replicas {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("deployment %s in %s has not reached %d replicas after %d tries", targetName, namespace, replicas, tries)
}

func WaitForStatefulSetReplicas(clientset *kubernetes.Clientset, namespace string, targetName string, replicas int32, tries int) error {
	for i := 0; i < tries; i++ {
		scale, _ := clientset.AppsV1().StatefulSets(namespace).GetScale(context.TODO(), targetName, metav1.GetOptions{})
		if scale.Status.Replicas == replicas {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("statefulset %s in %s has not reached %d replicas after %d tries", targetName, namespace, replicas, tries)
}
