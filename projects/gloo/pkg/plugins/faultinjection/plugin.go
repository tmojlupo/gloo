package faultinjection

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	envoyhttpfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var pluginStage = plugins.DuringStage(plugins.FaultStage)

var _ plugins.Plugin = &Plugin{}
var _ plugins.HttpFilterPlugin = &Plugin{}
var _ plugins.RoutePlugin = &Plugin{}

type Plugin struct {
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	// put the filter in the chain, but the actual faults will be configured on the routes
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(wellknown.Fault, pluginStage),
	}, nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	markFilterConfigFunc := func(spec *v1.Destination) (proto.Message, error) {
		if in.Options == nil {
			return nil, nil
		}
		routeFaults := in.GetOptions().GetFaults()
		if routeFaults == nil {
			return nil, nil
		}
		routeAbort := routeFaults.GetAbort()
		routeDelay := routeFaults.GetDelay()
		if routeAbort == nil && routeDelay == nil {
			return nil, nil
		}
		return generateEnvoyConfigForHttpFault(routeAbort, routeDelay), nil
	}
	return pluginutils.MarkPerFilterConfig(params.Ctx, params.Snapshot, in, out, wellknown.Fault, markFilterConfigFunc)
}

func toEnvoyAbort(abort *fault.RouteAbort) *envoyhttpfault.FaultAbort {
	if abort == nil {
		return nil
	}
	percentage := common.ToEnvoyPercentage(abort.Percentage)
	errorType := &envoyhttpfault.FaultAbort_HttpStatus{
		HttpStatus: uint32(abort.HttpStatus),
	}
	return &envoyhttpfault.FaultAbort{
		Percentage: percentage,
		ErrorType:  errorType,
	}
}

func toEnvoyDelay(delay *fault.RouteDelay) *envoyfault.FaultDelay {
	if delay == nil {
		return nil
	}
	percentage := common.ToEnvoyPercentage(delay.Percentage)
	delaySpec := &envoyfault.FaultDelay_FixedDelay{
		FixedDelay: delay.FixedDelay,
	}
	return &envoyfault.FaultDelay{
		Percentage:         percentage,
		FaultDelaySecifier: delaySpec,
	}
}

func generateEnvoyConfigForHttpFault(routeAbort *fault.RouteAbort, routeDelay *fault.RouteDelay) *envoyhttpfault.HTTPFault {
	abort := toEnvoyAbort(routeAbort)
	delay := toEnvoyDelay(routeDelay)
	return &envoyhttpfault.HTTPFault{
		Abort: abort,
		Delay: delay,
	}
}
