package main

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFindTargetForService(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock service with selector
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-namespace",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Create mock pods
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.CoreV1().Services("test-namespace").Create(context.TODO(), service, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-service",
		Namespace: "test-namespace",
		Kind:      resourceTypeSvc,
		Port:      8080,
	}
	
	updatedTarget, err := findTargetForServiceWithClient(realClient, res)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if updatedTarget.Name != "test-pod" {
		t.Errorf("Expected target name 'test-pod', got: %s", updatedTarget.Name)
	}
	
	if updatedTarget.Kind != resourceTypePod {
		t.Errorf("Expected target kind 'pods', got: %s", updatedTarget.Kind)
	}
}

func TestFindTargetForServiceNoPods(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock service with selector
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-namespace",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "nonexistent-app",
			},
		},
	}
	
	// Create a pod with different labels
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "different-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.CoreV1().Services("test-namespace").Create(context.TODO(), service, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-service",
		Namespace: "test-namespace",
		Kind:      resourceTypeSvc,
		Port:      8080,
	}
	
	_, err := findTargetForServiceWithClient(realClient, res)
	if err == nil {
		t.Errorf("Expected error when no matching pods found, got nil")
	}
}

func TestParseKubernetesServiceURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expectedNamespace string
		expectedName      string
		expectedKind      resourceType
		expectedPort      int
		hasError bool
	}{
		{
			name: "valid service URL",
			url:  "http://my-service.default.svc:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-service",
			expectedKind:      resourceTypeSvc,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid pod URL",
			url:  "http://my-pod.default.pod:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-pod",
			expectedKind:      resourceTypePod,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid deployment URL (deploy)",
			url:  "http://my-deployment.default.deploy:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-deployment",
			expectedKind:      resourceTypeDeployment,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid deployment URL (deployment)",
			url:  "http://my-deployment.default.deployment:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-deployment",
			expectedKind:      resourceTypeDeployment,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid statefulset URL (sts)",
			url:  "http://my-app.default.sts:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-app",
			expectedKind:      resourceTypeStatefulSet,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid statefulset URL (statefulset)",
			url:  "http://my-app.default.statefulset:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-app",
			expectedKind:      resourceTypeStatefulSet,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid daemonset URL (ds)",
			url:  "http://my-daemon.default.ds:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-daemon",
			expectedKind:      resourceTypeDaemonSet,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "valid replicaset URL (rs)",
			url:  "http://my-rs.default.rs:8080/api",
			expectedNamespace: "default",
			expectedName:      "my-rs",
			expectedKind:      resourceTypeReplicaSet,
			expectedPort:      8080,
			hasError: false,
		},
		{
			name: "service URL with cluster.local",
			url:  "http://my-service.default.svc.cluster.local:9090/api",
			expectedNamespace: "default",
			expectedName:      "my-service",
			expectedKind:      resourceTypeSvc,
			expectedPort:      9090,
			hasError: false,
		},
		{
			name:     "invalid URL format",
			url:      "http://invalid-url",
			expectedNamespace: "",
			expectedName:      "",
			expectedKind:      "",
			expectedPort:      0,
			hasError: true,
		},
		{
			name:     "unsupported resource type",
			url:      "http://my-unknown.default.job:8080/api",
			expectedNamespace: "",
			expectedName:      "",
			expectedKind:      "",
			expectedPort:      0,
			hasError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseKubernetesServiceURL(tc.url)

			if tc.hasError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tc.hasError {
				if result.namespace != tc.expectedNamespace {
					t.Errorf("Expected namespace %s, got %s", tc.expectedNamespace, result.namespace)
				}
				if result.name != tc.expectedName {
					t.Errorf("Expected name %s, got %s", tc.expectedName, result.name)
				}
				if result.kind != tc.expectedKind {
					t.Errorf("Expected kind %s, got %s", tc.expectedKind, result.kind)
				}
				if result.port != tc.expectedPort {
					t.Errorf("Expected port %d, got %d", tc.expectedPort, result.port)
				}
			}
		})
	}
}

func TestFindTargetForDeployment(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock deployment with selector
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
		},
	}
	
	// Create mock pods
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.AppsV1().Deployments("test-namespace").Create(context.TODO(), deployment, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Kind:      resourceTypeDeployment,
		Port:      8080,
	}
	
	updatedTarget, err := findTargetForServiceWithClient(realClient, res)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if updatedTarget.Name != "test-pod" {
		t.Errorf("Expected target name 'test-pod', got: %s", updatedTarget.Name)
	}
	
	if updatedTarget.Kind != resourceTypePod {
		t.Errorf("Expected target kind 'pods', got: %s", updatedTarget.Kind)
	}
}

func TestFindTargetForStatefulSet(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock statefulset with selector
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "test-namespace",
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
		},
	}
	
	// Create mock pods
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.AppsV1().StatefulSets("test-namespace").Create(context.TODO(), statefulset, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-statefulset",
		Namespace: "test-namespace",
		Kind:      resourceTypeStatefulSet,
		Port:      8080,
	}
	
	updatedTarget, err := findTargetForServiceWithClient(realClient, res)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if updatedTarget.Name != "test-pod" {
		t.Errorf("Expected target name 'test-pod', got: %s", updatedTarget.Name)
	}
	
	if updatedTarget.Kind != resourceTypePod {
		t.Errorf("Expected target kind 'pods', got: %s", updatedTarget.Kind)
	}
}

func TestFindTargetForDaemonSet(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock daemonset with selector
	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "test-namespace",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
		},
	}
	
	// Create mock pods
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.AppsV1().DaemonSets("test-namespace").Create(context.TODO(), daemonset, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-daemonset",
		Namespace: "test-namespace",
		Kind:      resourceTypeDaemonSet,
		Port:      8080,
	}
	
	updatedTarget, err := findTargetForServiceWithClient(realClient, res)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if updatedTarget.Name != "test-pod" {
		t.Errorf("Expected target name 'test-pod', got: %s", updatedTarget.Name)
	}
	
	if updatedTarget.Kind != resourceTypePod {
		t.Errorf("Expected target kind 'pods', got: %s", updatedTarget.Kind)
	}
}

func TestFindTargetForReplicaSet(t *testing.T) {
	// Create a fake Kubernetes client
	clientset := fake.NewSimpleClientset()
	
	// Create a mock replicaset with selector
	replicaset := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-replicaset",
			Namespace: "test-namespace",
		},
		Spec: appsv1.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
		},
	}
	
	// Create mock pods
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-app",
			},
		},
	}
	
	// Add them to the fake client
	_, _ = clientset.AppsV1().ReplicaSets("test-namespace").Create(context.TODO(), replicaset, metav1.CreateOptions{})
	_, _ = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), pod, metav1.CreateOptions{})
	
	// Test the function using the real client wrapper
	realClient := &RealKubeClient{clientset: clientset}
	
	// Test the function
	res := &ForwardTarget{
		Name:      "test-replicaset",
		Namespace: "test-namespace",
		Kind:      resourceTypeReplicaSet,
		Port:      8080,
	}
	
	updatedTarget, err := findTargetForServiceWithClient(realClient, res)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if updatedTarget.Name != "test-pod" {
		t.Errorf("Expected target name 'test-pod', got: %s", updatedTarget.Name)
	}
	
	if updatedTarget.Kind != resourceTypePod {
		t.Errorf("Expected target kind 'pods', got: %s", updatedTarget.Kind)
	}
}