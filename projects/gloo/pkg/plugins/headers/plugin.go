package headers

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	MissingHeaderValueError = errors.Errorf("header section of header value option cannot be nil")
)

// Puts Header Manipulation config on Routes, VirtualHosts, and Weighted Clusters
type Plugin struct{}

var _ plugins.RoutePlugin = NewPlugin()
var _ plugins.VirtualHostPlugin = NewPlugin()
var _ plugins.WeightedDestinationPlugin = NewPlugin()

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessWeightedDestination(params plugins.RouteParams, in *v1.WeightedDestination, out *envoyroute.WeightedCluster_ClusterWeight) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()
	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

type envoyHeaderManipulation struct {
	RequestHeadersToAdd     []*envoycore.HeaderValueOption
	RequestHeadersToRemove  []string
	ResponseHeadersToAdd    []*envoycore.HeaderValueOption
	ResponseHeadersToRemove []string
}

func getSecretsFromSnapshot(snapshot *v1.ApiSnapshot) *v1.SecretList {
	var secrets *v1.SecretList
	if snapshot == nil {
		secrets = &v1.SecretList{}
	} else {
		secrets = &snapshot.Secrets
	}
	return secrets
}

func convertHeaderConfig(in *headers.HeaderManipulation, secrets *v1.SecretList) (*envoyHeaderManipulation, error) {
	// request headers can either be made from a normal key/value pair, or.
	// they can be constructed from a supplied secret. To accomplish this, we use
	// a utility function that was originally created to accomplish this for health check headers.
	requestAdd, err := gogoutils.ToEnvoyHeaderValueOptionList(in.GetRequestHeadersToAdd(), secrets)
	if err != nil {
		return nil, err
	}
	// response headers have no reason to include secrets.
	responseAdd, err := convertResponseHeaderValueOption(in.GetResponseHeadersToAdd())
	if err != nil {
		return nil, err
	}

	return &envoyHeaderManipulation{
		RequestHeadersToAdd:     requestAdd,
		RequestHeadersToRemove:  in.GetRequestHeadersToRemove(),
		ResponseHeadersToAdd:    responseAdd,
		ResponseHeadersToRemove: in.GetResponseHeadersToRemove(),
	}, nil
}

func convertResponseHeaderValueOption(in []*headers.HeaderValueOption) ([]*envoycore.HeaderValueOption, error) {
	var out []*envoycore.HeaderValueOption
	for _, h := range in {
		if h.Header == nil {
			return nil, MissingHeaderValueError
		}
		out = append(out, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   h.Header.Key,
				Value: h.Header.Value,
			},
			Append: gogoutils.BoolGogoToProto(h.Append),
		})
	}
	return out, nil
}
