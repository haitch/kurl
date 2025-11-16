package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type resourceType string

const resourceTypePod resourceType = "pods"
const resourceTypeSvc resourceType = "services"
const resourceTypeDeployment resourceType = "deployments"
const resourceTypeStatefulSet resourceType = "statefulsets"
const resourceTypeDaemonSet resourceType = "daemonsets"
const resourceTypeReplicaSet resourceType = "replicasets"

var resourceNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type forwardTarget struct {
	namespace string
	name      string
	kind      resourceType
	port      int
}

func (res *forwardTarget) String() string {
	return fmt.Sprintf("namespaces=%s, name=%s, type=%s, port=%d", res.namespace, res.name, string(res.kind), res.port)
}

// parseKubernetesServiceURL extracts namespace, service name, and port from a Kubernetes service URL
func parseKubernetesServiceURL(rawURL string) (*forwardTarget, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	// Expected format: http://service.namespace.svc:port or http://service.namespace.svc.cluster.local:port
	host := strings.Split(parsedURL.Host, ":")[0]

	parts := strings.Split(host, ".")

	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid Kubernetes service URL format: %s", rawURL)
	}

	// Extract port
	port := 80
	portStr := parsedURL.Port()
	if portStr == "" {
		// Use default port based on scheme if no port specified
		if parsedURL.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	} else {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port in URL: %v", err)
		}
	}

	// Extract service name and namespace
	resourceName := parts[0]
	namespace := parts[1]
	var kind resourceType
	if parts[2] == "svc" {
		kind = resourceTypeSvc
	} else if parts[2] == "pod" {
		kind = resourceTypePod
	} else if parts[2] == "deploy" || parts[2] == "deployment" {
		kind = resourceTypeDeployment
	} else if parts[2] == "sts" || parts[2] == "statefulset" {
		kind = resourceTypeStatefulSet
	} else if parts[2] == "ds" || parts[2] == "daemonset" {
		kind = resourceTypeDaemonSet
	} else if parts[2] == "rs" || parts[2] == "replicaset" {
		kind = resourceTypeReplicaSet
	} else {
		return nil, fmt.Errorf("unsupported resource type: %s (supported: svc, pod, deploy, deployment, sts, statefulset, ds, daemonset, rs, replicaset)", parts[2])
	}

	// Basic validation for service name and namespace

	if !resourceNameRegex.MatchString(resourceName) {
		return nil, fmt.Errorf("invalid resource name: %s", resourceName)
	}

	if !resourceNameRegex.MatchString(namespace) {
		return nil, fmt.Errorf("invalid namespace: %s", namespace)
	}

	return &forwardTarget{namespace, resourceName, kind, port}, nil
}

// getKubernetesClient creates a Kubernetes client using the current kubeconfig context
func getKubernetesClient() (*kubernetes.Clientset, error) {
	// Build the client config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %v", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	return clientset, nil
}

// findFreePort finds an available local port to use for port-forwarding
func findFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

// Interface to abstract Kubernetes client operations for testing
type KubeClient interface {
	GetService(namespace, name string) (*corev1.Service, error)
	GetDeployment(namespace, name string) (*appsv1.Deployment, error)
	GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error)
	GetDaemonSet(namespace, name string) (*appsv1.DaemonSet, error)
	GetReplicaSet(namespace, name string) (*appsv1.ReplicaSet, error)
	ListPods(namespace string, selector labels.Selector) (*corev1.PodList, error)
}

// Implementation of KubeClient using real Kubernetes client
type RealKubeClient struct {
	clientset kubernetes.Interface
}

func (r *RealKubeClient) GetService(namespace, name string) (*corev1.Service, error) {
	return r.clientset.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r *RealKubeClient) GetDeployment(namespace, name string) (*appsv1.Deployment, error) {
	return r.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r *RealKubeClient) GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error) {
	return r.clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r *RealKubeClient) GetDaemonSet(namespace, name string) (*appsv1.DaemonSet, error) {
	return r.clientset.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r *RealKubeClient) GetReplicaSet(namespace, name string) (*appsv1.ReplicaSet, error) {
	return r.clientset.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (r *RealKubeClient) ListPods(namespace string, selector labels.Selector) (*corev1.PodList, error) {
	return r.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
}

// ForwardTarget represents the target for port forwarding
type ForwardTarget struct {
	Name      string
	Namespace string
	Kind      resourceType
	Port      int
}

// findTargetForService finds a pod that matches the service's selector
func findTargetForService(clientset *kubernetes.Clientset, res *ForwardTarget) (*ForwardTarget, error) {
	// Create real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	return findTargetForServiceWithClient(realClient, res)
}

// findTargetForServiceWithClient finds a pod that matches the resource's selector with a client interface
func findTargetForServiceWithClient(client KubeClient, res *ForwardTarget) (*ForwardTarget, error) {
	var selector labels.Selector
	var err error

	switch res.Kind {
	case resourceTypeSvc:
		// Get the service to find its selectors
		service, err := client.GetService(res.Namespace, res.Name)
		if err != nil {
			return res, fmt.Errorf("failed to get service %s in namespace %s: %v", res.Name, res.Namespace, err)
		}
		selector = labels.Set(service.Spec.Selector).AsSelector()
	case resourceTypeDeployment:
		// Get the deployment to find its selectors
		deployment, err := client.GetDeployment(res.Namespace, res.Name)
		if err != nil {
			return res, fmt.Errorf("failed to get deployment %s in namespace %s: %v", res.Name, res.Namespace, err)
		}
		selector, err = metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
		if err != nil {
			return res, fmt.Errorf("failed to convert deployment selector to labels selector: %v", err)
		}
	case resourceTypeStatefulSet:
		// Get the statefulset to find its selectors
		statefulset, err := client.GetStatefulSet(res.Namespace, res.Name)
		if err != nil {
			return res, fmt.Errorf("failed to get statefulset %s in namespace %s: %v", res.Name, res.Namespace, err)
		}
		selector, err = metav1.LabelSelectorAsSelector(statefulset.Spec.Selector)
		if err != nil {
			return res, fmt.Errorf("failed to convert statefulset selector to labels selector: %v", err)
		}
	case resourceTypeDaemonSet:
		// Get the daemonset to find its selectors
		daemonset, err := client.GetDaemonSet(res.Namespace, res.Name)
		if err != nil {
			return res, fmt.Errorf("failed to get daemonset %s in namespace %s: %v", res.Name, res.Namespace, err)
		}
		selector, err = metav1.LabelSelectorAsSelector(daemonset.Spec.Selector)
		if err != nil {
			return res, fmt.Errorf("failed to convert daemonset selector to labels selector: %v", err)
		}
	case resourceTypeReplicaSet:
		// Get the replicaset to find its selectors
		replicaset, err := client.GetReplicaSet(res.Namespace, res.Name)
		if err != nil {
			return res, fmt.Errorf("failed to get replicaset %s in namespace %s: %v", res.Name, res.Namespace, err)
		}
		selector, err = metav1.LabelSelectorAsSelector(replicaset.Spec.Selector)
		if err != nil {
			return res, fmt.Errorf("failed to convert replicaset selector to labels selector: %v", err)
		}
	default:
		// For pods, no need to look up selectors
		return res, nil
	}

	// Get pods matching the resource's selector
	pods, err := client.ListPods(res.Namespace, selector)
	if err != nil {
		return res, fmt.Errorf("failed to list pods for %s %s: %v", string(res.Kind), res.Name, err)
	}

	if len(pods.Items) == 0 {
		return res, fmt.Errorf("no pods found for %s %s in namespace %s", string(res.Kind), res.Name, res.Namespace)
	}

	// Use the first matching pod
	targetName := pods.Items[0].GetName()
	fmt.Printf("Found matching pod: %s for %s: %s\n", targetName, string(res.Kind), res.Name)

	// Return an updated target
	updatedTarget := &ForwardTarget{
		Name:      targetName,
		Namespace: res.Namespace,
		Kind:      resourceTypePod,
		Port:      res.Port,
	}
	return updatedTarget, nil
}

// runPortForward starts a port-forward using the Kubernetes client
func runPortForward(res *ForwardTarget, localPort int, stopCh <-chan struct{}, readyCh chan struct{}) error {
	// Get the Kubernetes client
	clientset, err := getKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %v", err)
	}

	// If the resource is a service, find a pod that matches the service's selector
	target := res

	if res.Kind == resourceTypeSvc {
		updatedTarget, err := findTargetForService(clientset, res)
		if err != nil {
			return err
		}
		target = updatedTarget
	}

	// Get the REST config for the cluster
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %v", err)
	}
	restConfig.GroupVersion = &corev1.SchemeGroupVersion
	restConfig.APIPath = "/api"
	restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	// Create the round tripper and upgrader for the port-forward
	// SPDY is the standard transport for port-forward operations in Kubernetes
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create SPDY round tripper: %v", err)
	}
	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return fmt.Errorf("failed to RESTClientFor: %v", err)
	}
	url := restClient.Post().Resource(string(target.Kind)).Namespace(target.Namespace).Name(target.Name).SubResource("portforward").URL()

	fmt.Println(url)
	// Create the dialer for the port-forward using SPDY
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	// Prepare the ports to forward
	ports := []string{fmt.Sprintf("%d:%d", localPort, target.Port)}

	// Create the port-forwarder
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create port-forwarder: %v", err)
	}

	// Run the port-forward
	return fw.ForwardPorts()
}
