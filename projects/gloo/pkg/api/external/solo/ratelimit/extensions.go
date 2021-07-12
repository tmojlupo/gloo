package v1alpha1

import (
	"context"

	"github.com/rotisserie/eris"
	skratelimit "github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	rlv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	RateLimitConfigCrd = crd.NewCrd(
		"ratelimitconfigs",
		RateLimitConfigGVK.Group,
		RateLimitConfigGVK.Version,
		RateLimitConfigGVK.Kind,
		"rlc",
		false,
		&rlv1alpha1.RateLimitConfig{})
)

func (list RateLimitConfigList) AsInputResources() resources.InputResourceList {
	var ress resources.InputResourceList
	for _, rateLimitConfig := range list {
		ress = append(ress, rateLimitConfig)
	}
	return ress
}

// This object is used to report the status for skv2 resources. skv2 CRDs declare the `status` field as a
// [sub-resource](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#subresources).
// This means that the status cannot be updated via the normal write operations on the main resource that the solo-kit
// resource clients perform. To get around this, we delegate the status update operation to the
// `UpdateRateLimitConfigStatus` on the skv2 client. We only need to do this when using Kubernetes as a config storage.
type kubeReporterClient struct {
	skClient   clients.ResourceClient
	skv2Client rlv1alpha1.RateLimitConfigClient
}

func NewRateLimitClients(ctx context.Context, rcFactory factory.ResourceClientFactory) (RateLimitConfigClient, reporter.ReporterResourceClient, error) {
	rlClient, err := NewRateLimitConfigClient(ctx, rcFactory)
	if err != nil {
		return nil, nil, err
	}

	var reporterClient reporter.ReporterResourceClient
	switch typedFactory := rcFactory.(type) {
	case *factory.KubeResourceClientFactory:
		rlClientSet, err := rlv1alpha1.NewClientsetFromConfig(typedFactory.Cfg)
		if err != nil {
			return nil, nil, err
		}
		reporterClient = &kubeReporterClient{
			skClient:   rlClient.BaseClient(),
			skv2Client: rlClientSet.RateLimitConfigs(),
		}
	default:
		reporterClient = rlClient.BaseClient()
	}
	return rlClient, reporterClient, nil
}

func (c *kubeReporterClient) Kind() string {
	return c.skClient.Kind()
}

func (c *kubeReporterClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	return c.skClient.Read(namespace, name, opts)
}

func (c *kubeReporterClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	rlConfig, ok := resource.(*RateLimitConfig)
	if !ok {
		return nil, eris.Errorf("unexpected type: expected %T, got %T", &RateLimitConfig{}, resource)
	}
	if !opts.OverwriteExisting {
		// Reporters should never create resources
		return nil, eris.New("unexpected create operation")
	}

	baseRlConfig := rlv1alpha1.RateLimitConfig(rlConfig.RateLimitConfig)

	err := c.skv2Client.UpdateRateLimitConfigStatus(opts.Ctx, &baseRlConfig)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to update resource status")
	}

	return &RateLimitConfig{
		RateLimitConfig: skratelimit.RateLimitConfig(baseRlConfig),
	}, nil
}
