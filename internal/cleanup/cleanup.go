package cleanup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	ManagedByLabel = "app.konflux-ci.org/managed-by"
	RunIDLabel     = "app.konflux.org/run-id"
	ManagedByValue = "konflux-test"
)

var NamespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}

type NamespaceInfo struct {
	Name   string
	RunID  string
	Labels map[string]string
}

type NamespaceNameLister interface {
	List(context.Context) ([]string, error)
}

type NamespaceService struct {
	Dynamic      dynamic.Interface
	NameLister   NamespaceNameLister
	PollInterval time.Duration
}

type OCNamespaceNameLister struct {
	Command string
}

func (l OCNamespaceNameLister) List(ctx context.Context) ([]string, error) {
	command := strings.TrimSpace(l.Command)
	if command == "" {
		command = strings.TrimSpace(os.Getenv("KONFLUX_TEST_OC"))
	}
	if command == "" {
		command = "oc"
	}
	var stderr bytes.Buffer
	output, err := exec.CommandContext(ctx, command, "get", "namespaces", "-o", "name", "--no-headers").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr.Write(exitErr.Stderr)
		}
		return nil, fmt.Errorf("list namespaces with %s: %w: %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return parseNamespaceNames(output)
}

func parseNamespaceNames(output []byte) ([]string, error) {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		name = strings.TrimPrefix(name, "namespace/")
		name = strings.TrimPrefix(name, "namespaces/")
		if strings.ContainsAny(name, " \t") {
			return nil, fmt.Errorf("unexpected namespace listing line %q", line)
		}
		if _, found := seen[name]; found {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (s NamespaceService) Inspect(ctx context.Context, name string) (NamespaceInfo, error) {
	if s.Dynamic == nil {
		return NamespaceInfo{}, fmt.Errorf("dynamic client is required")
	}
	if name == "" {
		return NamespaceInfo{}, fmt.Errorf("namespace name is required")
	}
	object, err := s.Dynamic.Resource(NamespaceGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return NamespaceInfo{}, err
	}
	return namespaceInfo(object), nil
}

func (s NamespaceService) FindCandidates(ctx context.Context, target string) ([]NamespaceInfo, error) {
	if s.Dynamic == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("tenant namespace is required")
	}
	lister := s.NameLister
	if lister == nil {
		lister = OCNamespaceNameLister{}
	}
	names, err := lister.List(ctx)
	if err != nil {
		return nil, err
	}
	scopedNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name != target && !strings.HasPrefix(name, target+"-") {
			continue
		}
		if _, found := seen[name]; found {
			continue
		}
		seen[name] = struct{}{}
		scopedNames = append(scopedNames, name)
	}
	if len(scopedNames) == 0 {
		return nil, nil
	}
	owned, err := s.Dynamic.Resource(NamespaceGVR).List(ctx, metav1.ListOptions{LabelSelector: ManagedByLabel + "=" + ManagedByValue})
	if err != nil {
		return nil, fmt.Errorf("verify namespace ownership: %w", err)
	}
	ownedByName := make(map[string]unstructured.Unstructured, len(owned.Items))
	for _, object := range owned.Items {
		ownedByName[object.GetName()] = object
	}
	candidates := make([]NamespaceInfo, 0)
	for _, name := range scopedNames {
		object, found := ownedByName[name]
		if !found {
			return nil, fmt.Errorf("tenant namespace %s exists without exact konflux-test ownership labels; manual intervention required", name)
		}
		info := namespaceInfo(&object)
		if info.RunID == "" {
			return nil, fmt.Errorf("tenant namespace %s exists without exact konflux-test ownership labels; manual intervention required", name)
		}
		candidates = append(candidates, info)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name < candidates[j].Name })
	return candidates, nil
}

func (s NamespaceService) Delete(ctx context.Context, name, runID string) error {
	if runID == "" {
		return fmt.Errorf("run ID is required for namespace deletion")
	}
	info, err := s.Inspect(ctx, name)
	if err != nil {
		return fmt.Errorf("re-read namespace %s: %w", name, err)
	}
	if info.Name != name {
		return fmt.Errorf("namespace identity mismatch: expected %q, got %q", name, info.Name)
	}
	if info.Labels[ManagedByLabel] != ManagedByValue || info.RunID != runID {
		return fmt.Errorf("refusing to delete namespace %q without exact konflux-test ownership labels", name)
	}
	_, _ = StripAllFinalizers(ctx, s.Dynamic, name)
	if err := s.Dynamic.Resource(NamespaceGVR).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}
	return nil
}

func (s NamespaceService) WaitDeleted(ctx context.Context, name string) error {
	if s.Dynamic == nil {
		return fmt.Errorf("dynamic client is required")
	}
	interval := s.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		_, err := s.Dynamic.Resource(NamespaceGVR).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("wait for namespace %s deletion: %w", name, err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func namespaceInfo(object *unstructured.Unstructured) NamespaceInfo {
	labels := object.GetLabels()
	copyLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		copyLabels[key] = value
	}
	return NamespaceInfo{Name: object.GetName(), RunID: labels[RunIDLabel], Labels: copyLabels}
}
