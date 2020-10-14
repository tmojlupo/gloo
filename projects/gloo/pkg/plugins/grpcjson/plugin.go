package grpcjson

import (
	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// filter info
var pluginStage = plugins.BeforeStage(plugins.OutAuthStage)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	grpcJsonConf := listener.GetOptions().GetGrpcJsonTranscoder()
	if grpcJsonConf == nil {
		return nil, nil
	}

	envoyGrpcJsonConf, err := translateGlooToEnvoyGrpcJson(grpcJsonConf)
	if err != nil {
		return nil, err
	}

	grpcJsonFilter, err := plugins.NewStagedFilterWithConfig(wellknown.GRPCJSONTranscoder, envoyGrpcJsonConf, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{grpcJsonFilter}, nil
}

func translateGlooToEnvoyGrpcJson(grpcJsonConf *grpc_json.GrpcJsonTranscoder) (*envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder, error) {

	envoyGrpcJsonConf := &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
		DescriptorSet:                nil, // to be filled in later
		Services:                     grpcJsonConf.GetServices(),
		PrintOptions:                 translateGlooToEnvoyPrintOptions(grpcJsonConf.GetPrintOptions()),
		MatchIncomingRequestRoute:    grpcJsonConf.GetMatchIncomingRequestRoute(),
		IgnoredQueryParameters:       grpcJsonConf.GetIgnoredQueryParameters(),
		AutoMapping:                  grpcJsonConf.GetAutoMapping(),
		IgnoreUnknownQueryParameters: grpcJsonConf.GetIgnoreUnknownQueryParameters(),
		ConvertGrpcStatus:            grpcJsonConf.GetConvertGrpcStatus(),
	}

	switch typedDescriptorSet := grpcJsonConf.GetDescriptorSet().(type) {
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptor:
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: typedDescriptorSet.ProtoDescriptor}
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin:
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: typedDescriptorSet.ProtoDescriptorBin}
	}

	return envoyGrpcJsonConf, nil
}

func translateGlooToEnvoyPrintOptions(options *grpc_json.GrpcJsonTranscoder_PrintOptions) *envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_PrintOptions {
	if options == nil {
		return nil
	}
	return &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_PrintOptions{
		AddWhitespace:              options.GetAddWhitespace(),
		AlwaysPrintPrimitiveFields: options.GetAlwaysPrintPrimitiveFields(),
		AlwaysPrintEnumsAsInts:     options.GetAlwaysPrintEnumsAsInts(),
		PreserveProtoFieldNames:    options.GetPreserveProtoFieldNames(),
	}
}
