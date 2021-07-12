package translator

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

type Opts struct {
	GlooNamespace                 string
	WriteNamespace                string
	WatchNamespaces               []string
	Gateways                      factory.ResourceClientFactory
	VirtualServices               factory.ResourceClientFactory
	RouteTables                   factory.ResourceClientFactory
	Proxies                       factory.ResourceClientFactory
	RouteOptions                  factory.ResourceClientFactory
	VirtualHostOptions            factory.ResourceClientFactory
	WatchOpts                     clients.WatchOpts
	ValidationServerAddress       string
	DevMode                       bool
	ReadGatewaysFromAllNamespaces bool
	Validation                    *ValidationOpts
}

type ValidationOpts struct {
	ProxyValidationServerAddress string
	ValidatingWebhookPort        int
	ValidatingWebhookCertPath    string
	ValidatingWebhookKeyPath     string
	IgnoreProxyValidationFailure bool
	AlwaysAcceptResources        bool
	AllowWarnings                bool
	WarnOnRouteShortCircuiting   bool
}
