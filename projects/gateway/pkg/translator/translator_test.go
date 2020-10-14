package translator_test

import (
	"context"
	"time"

	"github.com/solo-io/go-utils/testutils"

	"github.com/hashicorp/go-multierror"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/test/samples"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ns  = "gloo-system"
	ns2 = "gloo-system2"
)

var _ = Describe("Translator", func() {
	var (
		snap       *v1.ApiSnapshot
		labelSet   = map[string]string{"a": "b"}
		translator Translator
	)

	Context("translator", func() {
		BeforeEach(func() {
			translator = NewTranslator([]ListenerFactory{&HttpTranslator{}, &TcpTranslator{}}, Opts{})
			snap = &v1.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
						BindPort: 2,
					},
					{
						Metadata: core.Metadata{Namespace: ns2, Name: "name2"},
						GatewayType: &v1.Gateway_HttpGateway{
							HttpGateway: &v1.HttpGateway{},
						},
						BindPort: 2,
					},
				},
				VirtualServices: v1.VirtualServiceList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name1"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d1.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/1",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d1",
										},
									},
								},
							},
						},
					},
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &v1.VirtualHost{
							Domains: []string{"d2.com"},
							Routes: []*v1.Route{
								{
									Matchers: []*matchers.Matcher{{
										PathSpecifier: &matchers.Matcher_Prefix{
											Prefix: "/2",
										},
									}},
									Action: &v1.Route_DirectResponseAction{
										DirectResponseAction: &gloov1.DirectResponseAction{
											Body: "d2",
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should translate proxy with default name", func() {
			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(4))
			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
		})

		It("should properly translate listener plugins to proxy listener", func() {

			als := &als.AccessLoggingService{
				AccessLog: []*als.AccessLog{{
					OutputDestination: &als.AccessLog_FileSink{
						FileSink: &als.FileSink{
							Path: "/test",
						}},
				}},
			}
			snap.Gateways[0].Options = &gloov1.ListenerOptions{
				AccessLoggingService: als,
			}

			httpGateway := snap.Gateways[0].GetHttpGateway()
			Expect(httpGateway).NotTo(BeNil())
			waf := &waf.Settings{
				CustomInterventionMessage: "custom",
			}
			httpGateway.Options = &gloov1.HttpListenerOptions{
				Waf: waf,
			}

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs).To(HaveLen(4))
			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
			Expect(proxy.Listeners[0].Options.AccessLoggingService).To(Equal(als))
			httpListener := proxy.Listeners[0].GetHttpListener()
			Expect(httpListener).NotTo(BeNil())
			Expect(httpListener.Options.Waf).To(Equal(waf))
		})

		It("should translate two gateways with same name (different types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v1.Gateway{
					Metadata: core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v1.Gateway_TcpGateway{
						TcpGateway: &v1.TcpGateway{},
					},
				},
			)

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(2))
		})

		It("should translate two gateways with same name (and types) to one proxy with the same name", func() {
			snap.Gateways = append(
				snap.Gateways,
				&v1.Gateway{
					Metadata: core.Metadata{Namespace: ns, Name: "name2"},
					GatewayType: &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			)

			proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(2))
		})

		It("should error on two gateways with the same port in the same namespace", func() {
			dupeGateway := v1.Gateway{
				Metadata: core.Metadata{Namespace: ns, Name: "name2"},
				BindPort: 2,
			}
			snap.Gateways = append(snap.Gateways, &dupeGateway)

			_, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
			err := errs.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bind-address :2 is not unique in a proxy. gateways: gloo-system.name,gloo-system.name2"))
		})

		It("should warn on vs with missing delegate action", func() {

			badRoute := &v1.Route{
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "don't",
								Namespace: "exist",
							},
						},
					},
				},
			}

			us := samples.SimpleUpstream()
			snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
			rt := snap.RouteTables[0]
			rt.Routes = append(rt.Routes, badRoute)

			_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
			err := reports.Validate()
			Expect(err).NotTo(HaveOccurred())
			err = reports.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("route table exist.don't missing"))
		})

		Context("when the gateway CRDs don't clash", func() {
			BeforeEach(func() {
				translator = NewTranslator([]ListenerFactory{&HttpTranslator{}, &TcpTranslator{}}, Opts{
					ReadGatewaysFromAllNamespaces: true,
				})
				snap = &v1.ApiSnapshot{
					Gateways: v1.GatewayList{
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
						{
							Metadata: core.Metadata{Namespace: ns2, Name: "name2"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 3,
						},
					},
					VirtualServices: v1.VirtualServiceList{
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name1"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d1.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/1",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d1",
											},
										},
									},
								},
							},
						},
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name2"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d2.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/2",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d2",
											},
										},
									},
								},
							},
						},
					},
				}
			})

			It("should have the same number of listeners as gateways in the cluster", func() {
				proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs).To(HaveLen(4))
				Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
				Expect(proxy.Metadata.Name).To(Equal(defaults.GatewayProxyName))
				Expect(proxy.Metadata.Namespace).To(Equal(ns))
				Expect(proxy.Listeners).To(HaveLen(2))
			})
		})
	})

	Context("http", func() {

		Context("all-in-one virtual service", func() {

			var (
				factory *HttpTranslator
			)

			BeforeEach(func() {
				factory = &HttpTranslator{}
				translator = NewTranslator([]ListenerFactory{factory}, Opts{})
				snap = &v1.ApiSnapshot{
					Gateways: v1.GatewayList{
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
						{
							Metadata: core.Metadata{Namespace: ns2, Name: "name2"},
							GatewayType: &v1.Gateway_HttpGateway{
								HttpGateway: &v1.HttpGateway{},
							},
							BindPort: 2,
						},
					},
					VirtualServices: v1.VirtualServiceList{
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name1", Labels: labelSet},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d1.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/1",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d1",
											},
										},
									},
								},
							},
						},
						{
							Metadata: core.Metadata{Namespace: ns, Name: "name2"},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d2.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/2",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d2",
											},
										},
									},
								},
							},
						},
						{
							Metadata: core.Metadata{Namespace: ns + "-other-namespace", Name: "name3", Labels: labelSet},
							VirtualHost: &v1.VirtualHost{
								Domains: []string{"d3.com"},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/3",
											},
										}},
										Action: &v1.Route_DirectResponseAction{
											DirectResponseAction: &gloov1.DirectResponseAction{
												Body: "d3",
											},
										},
									},
								},
							},
						},
					},
				}
			})

			It("should translate an empty gateway to have all virtual services", func() {

				proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener

				Expect(listener.VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
			})

			It("omitting matchers should default to '/' prefix matcher", func() {

				snap.VirtualServices[0].VirtualHost.Routes[0].Matchers = nil
				snap.VirtualServices[1].VirtualHost.Routes[0].Matchers = []*matchers.Matcher{}
				proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener

				Expect(listener.VirtualHosts).To(HaveLen(len(snap.VirtualServices)))
				Expect(listener.VirtualHosts[0].Routes[0].Matchers).To(HaveLen(1))
				Expect(listener.VirtualHosts[1].Routes[0].Matchers).To(HaveLen(1))
				Expect(listener.VirtualHosts[0].Routes[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
				Expect(listener.VirtualHosts[1].Routes[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
			})

			It("should have no ssl config", func() {
				proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(proxy.Listeners).To(HaveLen(1))
				Expect(proxy.Listeners[0].SslConfigurations).To(BeEmpty())
			})

			Context("with VirtualServices (refs)", func() {
				It("should translate a gateway to only have its virtual services", func() {
					snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{
							VirtualServices: []core.ResourceRef{snap.VirtualServices[0].Metadata.Ref()},
						},
					}

					proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy).NotTo(BeNil())
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
					Expect(listener.VirtualHosts).To(HaveLen(1))
				})

				It("can include a virtual service from some other namespace", func() {
					snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{
							VirtualServices: []core.ResourceRef{snap.VirtualServices[2].Metadata.Ref()},
						},
					}

					proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy).NotTo(BeNil())
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
					Expect(listener.VirtualHosts).To(HaveLen(1))
					Expect(listener.VirtualHosts[0].Domains).To(Equal(snap.VirtualServices[2].VirtualHost.Domains))
				})
			})

			Context("with VirtualServiceSelector", func() {
				It("should translate a gateway to only have its virtual services", func() {
					snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{
							VirtualServiceSelector: labelSet,
						},
					}

					proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy).NotTo(BeNil())
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
					Expect(listener.VirtualHosts).To(HaveLen(2))
				})

				It("should prevent a gateway from matching virtual services outside its own namespace if so configured", func() {
					snap.Gateways[0].GatewayType = &v1.Gateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{
							VirtualServiceSelector:   labelSet,
							VirtualServiceNamespaces: []string{"gloo-system"},
						},
					}

					proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy).NotTo(BeNil())
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
					Expect(listener.VirtualHosts).To(HaveLen(1))
					Expect(listener.VirtualHosts[0].Domains).To(Equal(snap.VirtualServices[0].VirtualHost.Domains))
				})

			})

			It("should not have vhosts with ssl", func() {
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

				proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.ValidateStrict()).NotTo(HaveOccurred())

				var vsWithoutSsl v1.VirtualServiceList
				for _, vs := range snap.VirtualServices {
					if vs.SslConfig == nil {
						vsWithoutSsl = append(vsWithoutSsl, vs)
					}
				}
				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(len(vsWithoutSsl)))
				Expect(listener.VirtualHosts[0].Name).To(ContainSubstring("name2"))
				Expect(listener.VirtualHosts[1].Name).To(ContainSubstring("name3"))
			})

			It("should not have vhosts without ssl", func() {
				snap.Gateways[0].Ssl = true
				snap.VirtualServices[0].SslConfig = new(gloov1.SslConfig)

				proxy, errs := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

				Expect(errs.ValidateStrict()).NotTo(HaveOccurred())

				Expect(proxy.Listeners).To(HaveLen(1))
				listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
				Expect(listener.VirtualHosts).To(HaveLen(1))
				Expect(listener.VirtualHosts[0].Name).To(ContainSubstring("name1"))
			})

			Context("validate domains", func() {
				BeforeEach(func() {
					snap.VirtualServices[1].VirtualHost.Domains = snap.VirtualServices[0].VirtualHost.Domains
				})

				It("should error when 2 virtual services linked to the same gateway have overlapping domains", func() {
					_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
					errs := reports.ValidateStrict()
					Expect(errs).To(HaveOccurred())

					multiErr, ok := errs.(*multierror.Error)
					Expect(ok).To(BeTrue())

					for _, expectedError := range []error{
						DomainInOtherVirtualServicesErr("d1.com", []string{"gloo-system.name1"}),
						DomainInOtherVirtualServicesErr("d1.com", []string{"gloo-system.name2"}),
						GatewayHasConflictingVirtualServicesErr([]string{"d1.com"}),
					} {
						Expect(multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(expectedError)))
					}
				})

				It("should error when 2 virtual services linked to the same gateway have empty domains", func() {
					snap.VirtualServices[1].VirtualHost.Domains = nil
					snap.VirtualServices[0].VirtualHost.Domains = nil

					_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
					errs := reports.ValidateStrict()
					Expect(errs).To(HaveOccurred())

					multiErr, ok := errs.(*multierror.Error)
					Expect(ok).To(BeTrue())

					for _, expectedError := range []error{
						DomainInOtherVirtualServicesErr("", []string{"gloo-system.name1"}),
						DomainInOtherVirtualServicesErr("", []string{"gloo-system.name2"}),
						GatewayHasConflictingVirtualServicesErr([]string{""}),
					} {
						Expect(multiErr.WrappedErrors()).To(ContainElement(testutils.HaveInErrorChain(expectedError)))
					}
				})

				It("should warn when a virtual services does not specify a virtual host", func() {
					snap.VirtualServices[0].VirtualHost = nil

					_, reports := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)
					Expect(reports.Validate()).NotTo(HaveOccurred())

					errs := reports.ValidateStrict()
					Expect(errs).To(HaveOccurred())

					Expect(errs.Error()).To(ContainSubstring(NoVirtualHostErr(snap.VirtualServices[0]).Error()))
				})
			})
		})

		Context("using RouteTables and delegation", func() {
			Context("valid configuration", func() {
				dur := time.Minute

				rootLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &types.StringValue{Value: "root route plugin"}}
				midLevelRoutePlugins := &gloov1.RouteOptions{Timeout: &dur}
				leafLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &types.StringValue{Value: "leaf level plugin"}}

				mergedMidLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: rootLevelRoutePlugins.PrefixRewrite, Timeout: &dur}
				mergedLeafLevelRoutePlugins := &gloov1.RouteOptions{PrefixRewrite: &types.StringValue{Value: "leaf level plugin"}, Timeout: midLevelRoutePlugins.Timeout}

				BeforeEach(func() {
					translator = NewTranslator([]ListenerFactory{&HttpTranslator{}}, Opts{})
					snap = &v1.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HttpGateway{
									HttpGateway: &v1.HttpGateway{},
								},
								BindPort: 2,
							},
						},
						VirtualServices: v1.VirtualServiceList{
							{
								Metadata: core.Metadata{Namespace: ns, Name: "name1"},
								VirtualHost: &v1.VirtualHost{
									Domains: []string{"d1.com"},
									Routes: []*v1.Route{
										{
											Name: "testRouteName",
											Matchers: []*matchers.Matcher{{
												PathSpecifier: &matchers.Matcher_Prefix{
													Prefix: "/a",
												},
											}},
											Action: &v1.Route_DelegateAction{
												DelegateAction: &v1.DelegateAction{
													DelegationType: &v1.DelegateAction_Ref{
														Ref: &core.ResourceRef{
															Name:      "delegate-1",
															Namespace: ns,
														},
													},
												},
											},
											Options: rootLevelRoutePlugins,
										},
									},
								},
							},
							{
								Metadata: core.Metadata{Namespace: ns, Name: "name2"},
								VirtualHost: &v1.VirtualHost{
									Domains: []string{"d2.com"},
									Routes: []*v1.Route{
										{
											Matchers: []*matchers.Matcher{{
												PathSpecifier: &matchers.Matcher_Prefix{
													Prefix: "/b",
												},
											}},
											Action: &v1.Route_DelegateAction{
												DelegateAction: &v1.DelegateAction{
													DelegationType: &v1.DelegateAction_Ref{
														Ref: &core.ResourceRef{
															Name:      "delegate-2",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
						RouteTables: []*v1.RouteTable{
							{
								Metadata: core.Metadata{
									Name:      "delegate-1",
									Namespace: ns,
								},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/a/1-upstream",
											},
										}},
										Action: &v1.Route_RouteAction{
											RouteAction: &gloov1.RouteAction{
												Destination: &gloov1.RouteAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: &core.ResourceRef{
																Name:      "my-upstream",
																Namespace: ns,
															},
														},
													},
												},
											},
										},
									},
									{
										Name: "delegate1Route2",
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/a/3-delegate",
											},
										}},
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-3",
														Namespace: ns,
													},
												},
											},
										},
										Options: midLevelRoutePlugins,
									},
								},
							},
							{
								Metadata: core.Metadata{
									Name:      "delegate-2",
									Namespace: ns,
								},
								Routes: []*v1.Route{
									{
										Name: "delegate2Route1",
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/b/2-upstream",
											},
										}},
										Action: &v1.Route_RouteAction{
											RouteAction: &gloov1.RouteAction{
												Destination: &gloov1.RouteAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: &core.ResourceRef{
																Name:      "my-upstream",
																Namespace: ns,
															},
														},
													},
												},
											},
										},
									},
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/b/2-upstream-plugin-override",
											},
										}},
										Action: &v1.Route_RouteAction{
											RouteAction: &gloov1.RouteAction{
												Destination: &gloov1.RouteAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: &core.ResourceRef{
																Name:      "my-upstream",
																Namespace: ns,
															},
														},
													},
												},
											},
										},
										Options: leafLevelRoutePlugins,
									},
								},
							},
							{
								Metadata: core.Metadata{
									Name:      "delegate-3",
									Namespace: ns,
								},
								Routes: []*v1.Route{
									{
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/a/3-delegate/upstream1",
											},
										}},
										Action: &v1.Route_RouteAction{
											RouteAction: &gloov1.RouteAction{
												Destination: &gloov1.RouteAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: &core.ResourceRef{
																Name:      "my-upstream",
																Namespace: ns,
															},
														},
													},
												},
											},
										},
									},
									{
										Name: "delegate3Route2",
										Matchers: []*matchers.Matcher{{
											PathSpecifier: &matchers.Matcher_Prefix{
												Prefix: "/a/3-delegate/upstream2",
											},
										}},
										Action: &v1.Route_RouteAction{
											RouteAction: &gloov1.RouteAction{
												Destination: &gloov1.RouteAction_Single{
													Single: &gloov1.Destination{
														DestinationType: &gloov1.Destination_Upstream{
															Upstream: &core.ResourceRef{
																Name:      "my-upstream",
																Namespace: ns,
															},
														},
													},
												},
											},
										},
										Options: leafLevelRoutePlugins,
									},
								},
							},
						},
					}
				})

				It("merges the vs and route tables to a single gloov1.VirtualHost", func() {
					proxy, errs := translator.Translate(context.TODO(), "", ns, snap, snap.Gateways)
					Expect(errs.ValidateStrict()).NotTo(HaveOccurred())
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
					Expect(listener.VirtualHosts).To(HaveLen(2))

					// hack to assert equality on Metadata
					// gomega.Equals does not like *types.Struct
					for i, vh := range listener.VirtualHosts {
						for j, route := range vh.Routes {
							routeMeta, err := SourceMetaFromStruct(route.Metadata)
							Expect(err).NotTo(HaveOccurred())
							Expect(routeMeta).To(Equal(expectedRouteMetadata(i, j)))
							// after asserting Metadata equality, zero it out
							route.Metadata = nil
						}
					}

					Expect(listener.VirtualHosts[0].Routes).To(Equal([]*gloov1.Route{
						{
							Name: "vs:name1_route:testRouteName_rt:delegate-1_route:<unnamed>",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/a/1-upstream",
								},
							}},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "my-upstream",
													Namespace: "gloo-system",
												},
											},
										},
									},
								},
							},
							Options: rootLevelRoutePlugins,
						},
						{
							Name: "vs:name1_route:testRouteName_rt:delegate-1_route:delegate1Route2_rt:delegate-3_route:<unnamed>",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/a/3-delegate/upstream1",
								},
							}},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "my-upstream",
													Namespace: "gloo-system",
												},
											},
										},
									},
								},
							},
							Options: mergedMidLevelRoutePlugins,
						},
						{
							Name: "vs:name1_route:testRouteName_rt:delegate-1_route:delegate1Route2_rt:delegate-3_route:delegate3Route2",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/a/3-delegate/upstream2",
								},
							}},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "my-upstream",
													Namespace: "gloo-system",
												},
											},
										},
									},
								},
							},
							Options: mergedLeafLevelRoutePlugins,
						},
					}))
					Expect(listener.VirtualHosts[1].Routes).To(Equal([]*gloov1.Route{
						{
							Name: "vs:name2_route:<unnamed>_rt:delegate-2_route:delegate2Route1",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/b/2-upstream",
								},
							}},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "my-upstream",
													Namespace: "gloo-system",
												},
											},
										},
									},
								},
							},
						},
						{
							Name: "",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/b/2-upstream-plugin-override",
								},
							}},
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name:      "my-upstream",
													Namespace: "gloo-system",
												},
											},
										},
									},
								},
							},
							Options: leafLevelRoutePlugins,
						},
					}))
				})

			})

			Context("delegation cycle", func() {
				BeforeEach(func() {
					translator = NewTranslator([]ListenerFactory{&HttpTranslator{}}, Opts{})
					snap = &v1.ApiSnapshot{
						Gateways: v1.GatewayList{
							{
								Metadata: core.Metadata{Namespace: ns, Name: "name"},
								GatewayType: &v1.Gateway_HttpGateway{
									HttpGateway: &v1.HttpGateway{},
								},
								BindPort: 2,
							},
						},
						VirtualServices: v1.VirtualServiceList{
							{
								Metadata: core.Metadata{Namespace: ns, Name: "has-a-cycle"},
								VirtualHost: &v1.VirtualHost{
									Domains: []string{"d1.com"},
									Routes: []*v1.Route{
										{
											Action: &v1.Route_DelegateAction{
												DelegateAction: &v1.DelegateAction{
													DelegationType: &v1.DelegateAction_Ref{
														Ref: &core.ResourceRef{
															Name:      "delegate-1",
															Namespace: ns,
														},
													},
												},
											},
										},
									},
								},
							},
						},
						RouteTables: []*v1.RouteTable{
							{
								Metadata: core.Metadata{
									Name:      "delegate-1",
									Namespace: ns,
								},
								Routes: []*v1.Route{
									{
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-2",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
							{
								Metadata: core.Metadata{
									Name:      "delegate-2",
									Namespace: ns,
								},
								Routes: []*v1.Route{
									{
										Action: &v1.Route_DelegateAction{
											DelegateAction: &v1.DelegateAction{
												DelegationType: &v1.DelegateAction_Ref{
													Ref: &core.ResourceRef{
														Name:      "delegate-1",
														Namespace: ns,
													},
												},
											},
										},
									},
								},
							},
						},
					}
				})
				It("detects cycle and returns error", func() {
					_, errs := translator.Translate(context.TODO(), "", ns, snap, snap.Gateways)
					err := errs.ValidateStrict()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cycle detected"))
				})
			})

		})
	})

	Context("tcp", func() {
		var (
			factory     *TcpTranslator
			idleTimeout time.Duration
			plugins     *gloov1.TcpListenerOptions
			tcpHost     *gloov1.TcpHost
		)
		BeforeEach(func() {
			factory = &TcpTranslator{}
			translator = NewTranslator([]ListenerFactory{factory}, Opts{})

			idleTimeout = 5 * time.Second
			plugins = &gloov1.TcpListenerOptions{
				TcpProxySettings: &tcp.TcpProxySettings{
					MaxConnectAttempts: &types.UInt32Value{Value: 10},
					IdleTimeout:        &idleTimeout,
				},
			}
			tcpHost = &gloov1.TcpHost{
				Name: "host-one",
				Destination: &gloov1.TcpHost_TcpAction{
					Destination: &gloov1.TcpHost_TcpAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "ug-name",
						},
					},
				},
			}

			snap = &v1.ApiSnapshot{
				Gateways: v1.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						GatewayType: &v1.Gateway_TcpGateway{
							TcpGateway: &v1.TcpGateway{
								Options:  plugins,
								TcpHosts: []*gloov1.TcpHost{tcpHost},
							},
						},
						BindPort: 2,
					},
				},
			}
		})

		It("can properly translate a tcp proxy", func() {
			proxy, _ := translator.Translate(context.Background(), defaults.GatewayProxyName, ns, snap, snap.Gateways)

			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_TcpListener).TcpListener
			Expect(listener.Options).To(Equal(plugins))
			Expect(listener.TcpHosts).To(HaveLen(1))
			Expect(listener.TcpHosts[0]).To(Equal(tcpHost))
		})

	})

})

var expectedRouteMetadatas = [][]*SourceMetadata{
	{
		{
			Sources: []SourceRef{
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-3",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-3",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "name1",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
	},
	{
		{
			Sources: []SourceRef{
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "name2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
		{
			Sources: []SourceRef{
				{
					ResourceRef: core.ResourceRef{
						Name:      "delegate-2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.RouteTable",
					ObservedGeneration: 0,
				},
				{
					ResourceRef: core.ResourceRef{
						Name:      "name2",
						Namespace: "gloo-system",
					},
					ResourceKind:       "*v1.VirtualService",
					ObservedGeneration: 0,
				},
			},
		},
	},
}

func expectedRouteMetadata(virtualHostIndex, routeIndex int) *SourceMetadata {
	return expectedRouteMetadatas[virtualHostIndex][routeIndex]
}
