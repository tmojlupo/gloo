package regexutils

import (
	"context"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func NewRegex(ctx context.Context, regex string) *envoy_type_matcher_v3.RegexMatcher {
	settings := settingsutil.MaybeFromContext(ctx)
	return NewRegexFromSettings(settings, regex)
}

func NewRegexFromSettings(settings *v1.Settings, regex string) *envoy_type_matcher_v3.RegexMatcher {
	var programsize *uint32
	if settings != nil {
		if max_size := settings.GetGloo().GetRegexMaxProgramSize(); max_size != nil {
			programsize = &max_size.Value
		}
	}
	return NewRegexWithProgramSize(regex, programsize)
}

func NewRegexWithProgramSize(regex string, programsize *uint32) *envoy_type_matcher_v3.RegexMatcher {

	var maxProgramSize *wrappers.UInt32Value
	if programsize != nil {
		maxProgramSize = &wrappers.UInt32Value{
			Value: *programsize,
		}
	}

	return &envoy_type_matcher_v3.RegexMatcher{
		EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
			GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{MaxProgramSize: maxProgramSize},
		},
		Regex: regex,
	}
}
