package cluster

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestRealClusterCheckSchedulingUsesControllerConfigurationWithoutArmNodes(t *testing.T) {
	replicas := int32(1)
	clients := kubernetesfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: MultiPlatformControllerDeployment, Namespace: MultiPlatformControllerNamespace, Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: MultiPlatformControllerConfigMap, Namespace: MultiPlatformControllerNamespace, Labels: map[string]string{MultiPlatformControllerConfigLabel: "hosts"}},
			Data: map[string]string{
				"dynamic-platforms":        "linux/amd64,linux/arm64",
				"dynamic.linux-amd64.type": "aws",
				"dynamic.linux-arm64.type": "aws",
			},
		},
	)
	result := (&RealCluster{Clients: &ClientSet{Kubernetes: clients}}).CheckScheduling(context.Background(), SchedulingSpec{
		Namespace:  MultiPlatformControllerNamespace,
		Deployment: MultiPlatformControllerDeployment,
		ConfigMap:  MultiPlatformControllerConfigMap,
		Platforms:  []string{"linux/amd64", "linux/arm64"},
	})
	if len(result) != 3 {
		t.Fatalf("checks = %#v", result)
	}
	for _, check := range result {
		if !check.Passed {
			t.Fatalf("check failed: %#v", check)
		}
	}
}

func TestRealClusterCheckSchedulingRejectsUnconfiguredPlatform(t *testing.T) {
	replicas := int32(1)
	clients := kubernetesfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: MultiPlatformControllerDeployment, Namespace: MultiPlatformControllerNamespace, Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: MultiPlatformControllerConfigMap, Namespace: MultiPlatformControllerNamespace, Labels: map[string]string{MultiPlatformControllerConfigLabel: "hosts"}},
			Data:       map[string]string{"dynamic-platforms": "linux/amd64", "dynamic.linux-amd64.type": "aws"},
		},
	)
	result := (&RealCluster{Clients: &ClientSet{Kubernetes: clients}}).CheckScheduling(context.Background(), SchedulingSpec{
		Namespace:  MultiPlatformControllerNamespace,
		Deployment: MultiPlatformControllerDeployment,
		ConfigMap:  MultiPlatformControllerConfigMap,
		Platforms:  []string{"linux/arm64"},
	})
	if len(result) != 2 || result[1].Passed || result[1].Details == "" {
		t.Fatalf("checks = %#v", result)
	}
}
