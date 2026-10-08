module github.com/redhat-appstudio/konflux-test

go 1.26

toolchain go1.26.0

require (
	github.com/google/go-github/v66 v66.0.0
	github.com/google/uuid v1.6.0
	github.com/onsi/ginkgo/v2 v2.33.0
	github.com/onsi/gomega v1.44.0
	github.com/xanzy/go-gitlab v0.114.0
	golang.org/x/sys v0.46.0
	k8s.io/api v0.35.2
	k8s.io/apimachinery v0.35.2
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/term v0.44.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20260127142750-a19766b6e2d4 // indirect
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
)

replace (
	github.com/antlr/antlr4 => github.com/antlr/antlr4 v0.0.0-20211106181442-e4c1a74c66bd
	github.com/apcera/gssapi => github.com/openshift/gssapi v0.0.0-20161010215902-5fb4217df13b
	github.com/docker/distribution => github.com/docker/distribution v2.8.2+incompatible
	github.com/docker/distribution/reference => github.com/distribution/reference v0.5.0
	github.com/docker/docker => github.com/docker/docker v23.0.7+incompatible
	k8s.io/api => k8s.io/api v0.34.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.34.2
	k8s.io/apiserver => k8s.io/apiserver v0.34.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.34.2
	k8s.io/client-go => k8s.io/client-go v0.34.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.34.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.34.2
	k8s.io/code-generator => k8s.io/code-generator v0.34.2
	k8s.io/component-base => k8s.io/component-base v0.34.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.34.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.34.2
	k8s.io/cri-api => k8s.io/cri-api v0.34.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.34.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.34.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.34.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.34.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.34.2
	k8s.io/kubectl => k8s.io/kubectl v0.34.2
	k8s.io/kubelet => k8s.io/kubelet v0.34.2
	k8s.io/kubernetes => k8s.io/kubernetes v1.34.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.34.2
	k8s.io/metrics => k8s.io/metrics v0.34.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.34.2
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.34.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.34.2
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.20.2
	vbom.ml/util => github.com/fvbommel/util v0.0.0-20180919145318-efcd4e0f9787
)

replace github.com/cyphar/filepath-securejoin => github.com/cyphar/filepath-securejoin v0.3.6

replace github.com/expr-lang/expr => github.com/expr-lang/expr v1.17.7

replace github.com/go-chi/chi/v5 => github.com/go-chi/chi/v5 v5.2.4

replace github.com/go-chi/chi => github.com/go-chi/chi/v5 v5.2.4
