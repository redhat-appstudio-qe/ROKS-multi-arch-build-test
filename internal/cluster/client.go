package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type AccessCheck struct {
	Name      string
	Resource  schema.GroupVersionResource
	Namespace string
}

type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

const (
	MultiPlatformControllerNamespace   = "multi-platform-controller"
	MultiPlatformControllerDeployment  = "multi-platform-controller"
	MultiPlatformControllerConfigMap   = "host-config"
	MultiPlatformControllerConfigLabel = "build.appstudio.redhat.com/multi-platform-config"
)

type SchedulingSpec struct {
	Namespace  string
	Deployment string
	ConfigMap  string
	Platforms  []string
}

type PipelineRunSelector struct {
	Labels map[string]string
}

type Cluster interface {
	Server(context.Context) (string, error)
	CheckAccess(context.Context, []AccessCheck) []CheckResult
	CheckScheduling(context.Context, SchedulingSpec) []CheckResult
}

type ClientSet struct {
	Config     *rest.Config
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
}

func NewClientSet(kubeconfig string) (*ClientSet, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return NewClientSetFromConfig(config)
}

func NewClientSetFromConfig(config *rest.Config) (*ClientSet, error) {
	if config == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	if config.Timeout == 0 {
		config = rest.CopyConfig(config)
		config.Timeout = 20 * time.Second
	}
	kubernetesClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	return &ClientSet{Config: config, Kubernetes: kubernetesClient, Dynamic: dynamicClient, Discovery: discoveryClient}, nil
}

type RealCluster struct {
	Clients *ClientSet
}

func (c *RealCluster) Server(context.Context) (string, error) {
	if c == nil || c.Clients == nil || c.Clients.Config == nil {
		return "", fmt.Errorf("cluster clients are not initialized")
	}
	return strings.TrimRight(c.Clients.Config.Host, "/"), nil
}

func (c *RealCluster) CheckAccess(ctx context.Context, checks []AccessCheck) []CheckResult {
	results := make([]CheckResult, 0, len(checks))
	for _, check := range checks {
		result := CheckResult{Name: check.Name}
		if c == nil || c.Clients == nil || c.Clients.Discovery == nil {
			result.Details = "discovery client is not initialized"
			results = append(results, result)
			continue
		}
		_, err := c.Clients.Discovery.ServerResourcesForGroupVersion(check.Resource.GroupVersion().String())
		result.Passed = err == nil
		if err != nil {
			result.Details = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (c *RealCluster) CheckScheduling(ctx context.Context, spec SchedulingSpec) []CheckResult {
	if c == nil || c.Clients == nil || c.Clients.Kubernetes == nil {
		return []CheckResult{{Name: "multi-platform-controller", Details: "kubernetes client is not initialized"}}
	}
	if strings.TrimSpace(spec.Namespace) == "" || strings.TrimSpace(spec.Deployment) == "" || strings.TrimSpace(spec.ConfigMap) == "" {
		return []CheckResult{{Name: "multi-platform-controller", Details: "controller namespace, deployment, and config map are required"}}
	}
	deployment, err := c.Clients.Kubernetes.AppsV1().Deployments(spec.Namespace).Get(ctx, spec.Deployment, metav1.GetOptions{})
	if err != nil {
		return []CheckResult{{Name: spec.Deployment, Details: fmt.Sprintf("get controller deployment: %v", err)}}
	}
	if !controllerDeploymentReady(deployment) {
		return []CheckResult{{Name: spec.Deployment, Details: fmt.Sprintf("controller deployment is not ready: generation=%d observed=%d ready=%d available=%d updated=%d", deployment.Generation, deployment.Status.ObservedGeneration, deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas)}}
	}
	configMap, err := c.Clients.Kubernetes.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, spec.ConfigMap, metav1.GetOptions{})
	if err != nil {
		return []CheckResult{{Name: spec.ConfigMap, Details: fmt.Sprintf("get controller host configuration: %v", err)}}
	}
	if strings.TrimSpace(configMap.Labels[MultiPlatformControllerConfigLabel]) == "" {
		return []CheckResult{{Name: spec.ConfigMap, Details: "controller host configuration is not labeled for multi-platform-controller"}}
	}
	results := []CheckResult{{Name: spec.Deployment, Passed: true, Details: "controller deployment is available"}}
	for _, platform := range spec.Platforms {
		result := CheckResult{Name: spec.Namespace + "/" + platform}
		mode := configuredPlatformMode(configMap.Data, platform)
		if mode == "" {
			result.Details = "platform is not configured in host-config"
			results = append(results, result)
			continue
		}
		if mode == "dynamic" || mode == "dynamic-pool" {
			prefix := "dynamic." + strings.ReplaceAll(platform, "/", "-") + "."
			if strings.TrimSpace(configMap.Data[prefix+"type"]) == "" {
				result.Details = fmt.Sprintf("%s platform has no provider type", mode)
				results = append(results, result)
				continue
			}
		}
		result.Passed = true
		result.Details = fmt.Sprintf("platform is configured via %s host scheduling", mode)
		results = append(results, result)
	}
	return results
}

func controllerDeploymentReady(deployment *appsv1.Deployment) bool {
	if deployment == nil {
		return false
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	return desired > 0 && deployment.Status.ObservedGeneration >= deployment.Generation && deployment.Status.ReadyReplicas >= desired && deployment.Status.AvailableReplicas >= desired && deployment.Status.UpdatedReplicas >= desired
}

func configuredPlatformMode(data map[string]string, platform string) string {
	for _, entry := range []struct {
		key  string
		mode string
	}{
		{key: "local-platforms", mode: "local"},
		{key: "dynamic-platforms", mode: "dynamic"},
		{key: "dynamic-pool-platforms", mode: "dynamic-pool"},
	} {
		for _, configured := range strings.Split(data[entry.key], ",") {
			if strings.TrimSpace(configured) == platform {
				return entry.mode
			}
		}
	}
	return ""
}
