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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// parseKubernetesServiceURL extracts namespace, service name, and port from a Kubernetes service URL
func parseKubernetesServiceURL(rawURL string) (namespace, serviceName string, port int, err error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid URL: %v", err)
	}

	// Expected format: http://service.namespace.svc:port or http://service.namespace.svc.cluster.local:port
	host := parsedURL.Host
	parts := strings.Split(host, ".")

	if len(parts) < 3 {
		return "", "", 0, fmt.Errorf("invalid Kubernetes service URL format: %s", rawURL)
	}

	// Extract port
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
			return "", "", 0, fmt.Errorf("invalid port in URL: %v", err)
		}
	}

	// Extract service name and namespace
	serviceName = parts[0]
	namespace = parts[1]

	// Basic validation for service name and namespace
	serviceNameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !serviceNameRegex.MatchString(serviceName) {
		return "", "", 0, fmt.Errorf("invalid service name: %s", serviceName)
	}

	if !serviceNameRegex.MatchString(namespace) {
		return "", "", 0, fmt.Errorf("invalid namespace: %s", namespace)
	}

	return namespace, serviceName, port, nil
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
func runPortForward(serviceName, namespace string, servicePort, localPort int, stopCh <-chan struct{}, readyCh chan struct{}) error {
	// Get the REST config for the cluster
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %v", err)
	}

	// Create the round tripper and upgrader for the port-forward
	// SPDY is the standard transport for port-forward operations in Kubernetes
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create SPDY round tripper: %v", err)
	}

	// Construct the URL for the port-forward request
	path := fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%d/portforward", namespace, serviceName, servicePort)
	hostIP := strings.TrimLeft(restConfig.Host, "htps:/")

	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}
	if restConfig.Insecure {
		serverURL.Scheme = "http"
	}

	// Create the dialer for the port-forward using SPDY
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &serverURL)

	// Prepare the ports to forward
	ports := []string{fmt.Sprintf("%d:%d", localPort, servicePort)}

	// Create the port-forwarder
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create port-forwarder: %v", err)
	}

	// Run the port-forward
	return fw.ForwardPorts()
}