package e2e_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gogo/protobuf/types"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/v1helpers"
)

var _ = Describe("Staged Transformation", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
		envoyPort     uint32
		up            *gloov1.Upstream
		proxy         *gloov1.Proxy
	)

	BeforeEach(func() {
		proxy = nil
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		envoyPort = defaults.HttpPort

		// this upstream doesn't need to exist - in fact, we want ext auth to fail.
		extauthn := &gloov1.Upstream{
			Metadata: core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UseHttp2: &types.BoolValue{Value: true},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &gloov1static.UpstreamSpec{
					Hosts: []*gloov1static.Host{{
						Addr: "127.2.3.4",
						Port: 1234,
					}},
				},
			},
		}

		ref := extauthn.Metadata.Ref()
		ns := defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: ns,
			NsToWatch: []string{"default", ns},
			Settings: &gloov1.Settings{
				Extauth: &extauthv1.Settings{
					ExtauthzServerRef: &ref,
				},
			},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableUds:     true,
				DisableFds:     true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		_, err = testClients.UpstreamClient.Write(extauthn, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		err = envoyInstance.RunWithRole(ns+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		up = tu.Upstream
		_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	setProxyWithModifier := func(et *transformation.TransformationStages, modifier func(*gloov1.VirtualHost)) {
		proxy = getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())
		vs := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener.
			VirtualHosts[0]
		vs.Options = &gloov1.VirtualHostOptions{
			StagedTransformations: et,
			Extauth: &extauthv1.ExtAuthExtension{
				Spec: &extauthv1.ExtAuthExtension_Disable{
					Disable: true,
				},
			},
		}
		if modifier != nil {
			modifier(vs)
		}
		var err error
		proxy, err = testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	}
	setProxy := func(et *transformation.TransformationStages) {
		setProxyWithModifier(et, nil)
	}

	Context("no auth", func() {

		TestUpstreamReachable := func() {
			v1helpers.TestUpstreamReachable(envoyPort, tu, nil)
		}
		It("should transform response", func() {
			setProxy(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						Matchers: []*matchers.HeaderMatcher{
							{
								Name:  ":status",
								Value: "200",
							},
						},
						ResponseTransformation: &envoytransformation.Transformation{
							TransformationType: &envoytransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
				// add regular response to see that the early one overrides it
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						Matchers: []*matchers.HeaderMatcher{
							{
								Name:  ":status",
								Value: "200",
							},
						},
						ResponseTransformation: &envoytransformation.Transformation{
							TransformationType: &envoytransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "regular-transformed",
										},
									},
								},
							},
						},
					}},
				},
			})
			TestUpstreamReachable()

			// send a request and expect it transformed!
			body := []byte("test")
			v1helpers.ExpectHttpOK(body, nil, envoyPort, "early-transformed")
		})

		It("should not transform when auth succeeds", func() {
			setProxy(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseCodeDetails: "ext_authz_error",
						ResponseTransformation: &envoytransformation.Transformation{
							TransformationType: &envoytransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
			})
			TestUpstreamReachable()

			// send a request and expect it transformed!
			body := []byte("test")
			v1helpers.ExpectHttpOK(body, nil, envoyPort, "test")
		})
	})

	Context("with auth", func() {
		TestUpstreamReachable := func() {
			Eventually(func() error {
				_, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/1", envoyPort))
				return err
			}, "30s", "1s").ShouldNot(HaveOccurred())
		}

		It("should transform response code details", func() {
			setProxyWithModifier(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseCodeDetails: "ext_authz_error",
						ResponseTransformation: &envoytransformation.Transformation{
							TransformationType: &envoytransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
			}, func(vs *gloov1.VirtualHost) {
				vs.Options.Extauth = &extauthv1.ExtAuthExtension{
					Spec: &extauthv1.ExtAuthExtension_CustomAuth{
						CustomAuth: &extauthv1.CustomAuth{},
					},
				}
			})
			TestUpstreamReachable()
			// send a request and expect it transformed!
			res, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/1", envoyPort))
			Expect(err).NotTo(HaveOccurred())

			body, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("early-transformed"))
		})
	})

})
