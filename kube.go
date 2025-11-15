package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
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

var resourceNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type resource struct {
	namespace string
	name      string
	kind      resourceType
	port      int
}

func (res *resource) String() string {
	return fmt.Sprintf("namespaces=%s, name=%s, type=%s, port=%d", res.namespace, res.name, string(res.kind), res.port)
}

// parseKubernetesServiceURL extracts namespace, service name, and port from a Kubernetes service URL
func parseKubernetesServiceURL(rawURL string) (*resource, error) {
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
	}
	if parts[2] == "pod" {
		kind = resourceTypePod
	}

	// Basic validation for service name and namespace

	if !resourceNameRegex.MatchString(resourceName) {
		return nil, fmt.Errorf("invalid service name: %s", resourceName)
	}

	if !resourceNameRegex.MatchString(namespace) {
		return nil, fmt.Errorf("invalid namespace: %s", namespace)
	}

	return &resource{namespace, resourceName, kind, port}, nil
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

// runPortForward starts a port-forward using the Kubernetes client
func runPortForward(res *resource, localPort int, stopCh <-chan struct{}, readyCh chan struct{}) error {
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
	url := restClient.Post().Resource(string(res.kind)).Namespace(res.namespace).Name(res.name).SubResource("portforward").URL()

	fmt.Println(url)
	// Create the dialer for the port-forward using SPDY
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	// Prepare the ports to forward
	ports := []string{fmt.Sprintf("%d:%d", localPort, res.port)}

	// Create the port-forwarder
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create port-forwarder: %v", err)
	}

	// Run the port-forward
	return fw.ForwardPorts()
}
