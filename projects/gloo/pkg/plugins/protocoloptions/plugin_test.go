package protocoloptions_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/protocoloptions"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	var (
		p      *protocoloptions.Plugin
		params plugins.Params
		out    *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		p = protocoloptions.NewPlugin()
		out = new(envoy_config_cluster_v3.Cluster)

	})
	Context("upstream", func() {
		It("should not use window sizes if UseHttp2 is not true", func() {
			falseVal := &v1.Upstream{
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 7777777},
				UseHttp2:                    &wrappers.BoolValue{Value: false},
			}
			nilVal := &v1.Upstream{
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 7777777},
			}
			var nilOptions *envoy_config_core_v3.Http2ProtocolOptions = nil

			err := p.ProcessUpstream(params, falseVal, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).To(Equal(nilOptions))

			err = p.ProcessUpstream(params, nilVal, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).To(Equal(nilOptions))
		})

		It("should not accept connection streams that are too small", func() {
			tooSmall := &v1.Upstream{
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 65534},
				UseHttp2:                    &wrappers.BoolValue{Value: true},
			}

			err := p.ProcessUpstream(params, tooSmall, out)
			Expect(err).To(HaveOccurred())
		})

		It("should not accept connection streams that are too large", func() {
			tooBig := &v1.Upstream{
				InitialStreamWindowSize: &wrappers.UInt32Value{Value: 2147483648},
				UseHttp2:                &wrappers.BoolValue{Value: true},
			}
			err := p.ProcessUpstream(params, tooBig, out)
			Expect(err).To(HaveOccurred())
		})

		It("should accept connection streams that are within the correct range", func() {
			validUpstream := &v1.Upstream{
				InitialStreamWindowSize:     &wrappers.UInt32Value{Value: 268435457},
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 65535},
				UseHttp2:                    &wrappers.BoolValue{Value: true},
			}

			err := p.ProcessUpstream(params, validUpstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).NotTo(BeNil())
			Expect(out.Http2ProtocolOptions.InitialStreamWindowSize).To(Equal(&wrappers.UInt32Value{Value: 268435457}))
			Expect(out.Http2ProtocolOptions.InitialConnectionWindowSize).To(Equal(&wrappers.UInt32Value{Value: 65535}))
		})
	})
})
