package cluster

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
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

type PipelineRunSelector struct {
	Labels map[string]string
}

type ArchiveEndpoint struct {
	APIURL          string
	GVR             schema.GroupVersionResource
	NamespaceScoped bool
}

type Cluster interface {
	Server(context.Context) (string, error)
	CheckAccess(context.Context, []AccessCheck) []CheckResult
	DiscoverArchive(context.Context, schema.GroupVersionResource) (ArchiveEndpoint, error)
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

func (c *RealCluster) DiscoverArchive(ctx context.Context, gvr schema.GroupVersionResource) (ArchiveEndpoint, error) {
	if c == nil || c.Clients == nil || c.Clients.Discovery == nil {
		return ArchiveEndpoint{}, fmt.Errorf("discovery client is not initialized")
	}
	resources, err := c.Clients.Discovery.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		return ArchiveEndpoint{}, err
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return ArchiveEndpoint{GVR: gvr, NamespaceScoped: !resource.Namespaced}, nil
		}
	}
	return ArchiveEndpoint{}, fmt.Errorf("archive resource %s not discovered", gvr.String())
}

var _ meta.RESTMapper = nil
