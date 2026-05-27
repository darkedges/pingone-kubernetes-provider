// Package helm provides utilities for managing Helm releases within a Kubernetes operator.
package helm

import (
	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// NewHelmClient initialises a Helm action.Configuration scoped to the given namespace
// using an in-cluster REST config for storage and API access.
func NewHelmClient(namespace string, restConfig *rest.Config) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	getter := &restClientGetter{restConfig: restConfig, namespace: namespace}
	// logf is a no-op; structured logging happens in the reconciler.
	if err := cfg.Init(getter, namespace, "secrets", func(string, ...interface{}) {}); err != nil {
		return nil, err
	}
	return cfg, nil
}

// restClientGetter adapts a *rest.Config to the genericclioptions.RESTClientGetter interface
// required by the Helm action package.
type restClientGetter struct {
	restConfig *rest.Config
	namespace  string
}

func (r *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

func (r *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(r.restConfig)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (r *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

func (r *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	apiCfg := clientcmdapi.NewConfig()
	apiCfg.Clusters["in-cluster"] = &clientcmdapi.Cluster{Server: r.restConfig.Host}
	apiCfg.AuthInfos["in-cluster"] = &clientcmdapi.AuthInfo{Token: r.restConfig.BearerToken}
	apiCfg.Contexts["in-cluster"] = &clientcmdapi.Context{
		Cluster:   "in-cluster",
		AuthInfo:  "in-cluster",
		Namespace: r.namespace,
	}
	apiCfg.CurrentContext = "in-cluster"
	return clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{})
}
