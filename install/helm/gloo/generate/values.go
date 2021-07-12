package generate

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	appsv1 "k8s.io/api/core/v1"
)

type HelmConfig struct {
	Config
	Global *Global `json:"global,omitempty"`
}

type Config struct {
	Namespace      *Namespace              `json:"namespace,omitempty"`
	Settings       *Settings               `json:"settings,omitempty"`
	Gloo           *Gloo                   `json:"gloo,omitempty"`
	Discovery      *Discovery              `json:"discovery,omitempty"`
	Gateway        *Gateway                `json:"gateway,omitempty"`
	GatewayProxies map[string]GatewayProxy `json:"gatewayProxies,omitempty"`
	Ingress        *Ingress                `json:"ingress,omitempty"`
	IngressProxy   *IngressProxy           `json:"ingressProxy,omitempty"`
	K8s            *K8s                    `json:"k8s,omitempty"`
	AccessLogger   *AccessLogger           `json:"accessLogger,omitempty"`
}

type Global struct {
	Image             *Image           `json:"image,omitempty"`
	Extensions        interface{}      `json:"extensions,omitempty"`
	GlooRbac          *Rbac            `json:"glooRbac,omitempty"`
	GlooStats         Stats            `json:"glooStats,omitempty" desc:"Config used as the default values for Prometheus stats published from Gloo Edge pods. Can be overridden by individual deployments"`
	GlooMtls          Mtls             `json:"glooMtls,omitempty" desc:"Config used to enable internal mtls authentication"`
	IstioSDS          IstioSDS         `json:"istioSDS,omitempty" desc:"Config used for installing Gloo Edge with Istio SDS cert rotation features to facilitate Istio mTLS"`
	IstioIntegration  IstioIntegration `json:"istioIntegration,omitempty" desc:"Configs user to manage Gloo pod visibility for Istio's' automatic discovery and sidecar injection."`
	ExtraSpecs        *bool            `json:"extraSpecs,omitempty" desc:"Add additional specs to include in the settings manifest, as defined by a helm partial. Defaults to false in open source, and true in enterprise."`
	ExtauthCustomYaml *bool            `json:"extauthCustomYaml,omitempty" desc:"Inject whatever yaml exists in .Values.global.extensions.extAuth into settings.spec.extauth, instead of structured yaml (which is enterprise only). Defaults to true in open source, and false in enterprise"`
}

type Namespace struct {
	Create *bool `json:"create,omitempty" desc:"create the installation namespace"`
}

type Rbac struct {
	Create     *bool   `json:"create,omitempty" desc:"create rbac rules for the gloo-system service account"`
	Namespaced *bool   `json:"namespaced,omitempty" desc:"use Roles instead of ClusterRoles"`
	NameSuffix *string `json:"nameSuffix,omitempty" desc:"When nameSuffix is nonempty, append '-$nameSuffix' to the names of Gloo Edge RBAC resources; e.g. when nameSuffix is 'foo', the role 'gloo-resource-reader' will become 'gloo-resource-reader-foo'"`
}

// Common
type Image struct {
	Tag        *string `json:"tag,omitempty"  desc:"tag for the container"`
	Repository *string `json:"repository,omitempty"  desc:"image name (repository) for the container."`
	Registry   *string `json:"registry,omitempty" desc:"image prefix/registry e.g. (quay.io/solo-io)"`
	PullPolicy *string `json:"pullPolicy,omitempty"  desc:"image pull policy for the container"`
	PullSecret *string `json:"pullSecret,omitempty" desc:"image pull policy for the container "`
	Extended   *bool   `json:"extended,omitempty" desc:"if true, deploy an extended version of the container with additional debug tools"`
}

type ResourceAllocation struct {
	Memory *string `json:"memory,omitEmpty" desc:"amount of memory"`
	CPU    *string `json:"cpu,omitEmpty" desc:"amount of CPUs"`
}

type ResourceRequirements struct {
	Limits   *ResourceAllocation `json:"limits,omitEmpty" desc:"resource limits of this container"`
	Requests *ResourceAllocation `json:"requests,omitEmpty" desc:"resource requests of this container"`
}
type PodSpec struct {
	RestartPolicy *string                  `json:"restartPolicy,omitempty" desc:"restart policy to use when the pod exits"`
	NodeName      *string                  `json:"nodeName,omitempty" desc:"name of node to run on"`
	NodeSelector  map[string]string        `json:"nodeSelector,omitempty" desc:"label selector for nodes"`
	Tolerations   []*appsv1.Toleration     `json:"tolerations,omitEmpty"`
	Affinity      []map[string]interface{} `json:"affinity,omitempty"`
	HostAliases   []interface{}            `json:"hostAliases,omitempty"`
}

type JobSpec struct {
	*PodSpec
}

type DeploymentSpecSansResources struct {
	Replicas  *int             `json:"replicas,omitempty" desc:"number of instances to deploy"`
	CustomEnv []*appsv1.EnvVar `json:"customEnv,omitempty" desc:"custom extra environment variables for the container"`
	*PodSpec
}

type DeploymentSpec struct {
	DeploymentSpecSansResources
	Resources *ResourceRequirements `json:"resources,omitempty" desc:"resources for the main pod in the deployment"`
	*KubeResourceOverride
}

// Used to override any field in generated kubernetes resources.
type KubeResourceOverride struct {
	KubeResourceOverride map[string]interface{} `json:"kubeResourceOverride,omitempty" desc:"override fields in the generated resource by specifying the yaml structure to override under the top-level key."`
}

type Integrations struct {
	Knative                 *Knative                 `json:"knative,omitEmpty"`
	Consul                  *Consul                  `json:"consul,omitEmpty" desc:"Consul settings to inject into the consul client on startup"`
	ConsulUpstreamDiscovery *ConsulUpstreamDiscovery `json:"consulUpstreamDiscovery,omitEmpty" desc:"Settings for Gloo Edge's behavior when discovering consul services and creating upstreams for them."`
}

type Consul struct {
	Datacenter         *string                  `json:"datacenter,omitEmpty" desc:"Datacenter to use. If not provided, the default agent datacenter is used."`
	Username           *string                  `json:"username,omitEmpty" desc:"Username to use for HTTP Basic Authentication."`
	Password           *string                  `json:"password,omitEmpty" desc:"Password to use for HTTP Basic Authentication."`
	Token              *string                  `json:"token,omitEmpty" desc:"Token is used to provide a per-request ACL token which overrides the agent's default token."`
	CaFile             *string                  `json:"caFile,omitEmpty" desc:"caFile is the optional path to the CA certificate used for Consul communication, defaults to the system bundle if not specified."`
	CaPath             *string                  `json:"caPath,omitEmpty" desc:"caPath is the optional path to a directory of CA certificates to use for Consul communication, defaults to the system bundle if not specified."`
	CertFile           *string                  `json:"certFile,omitEmpty" desc:"CertFile is the optional path to the certificate for Consul communication. If this is set then you need to also set KeyFile."`
	KeyFile            *string                  `json:"keyFile,omitEmpty" desc:"KeyFile is the optional path to the private key for Consul communication. If this is set then you need to also set CertFile."`
	InsecureSkipVerify *bool                    `json:"insecureSkipVerify,omitEmpty" desc:"InsecureSkipVerify if set to true will disable TLS host verification."`
	WaitTime           *Duration                `json:"waitTime,omitEmpty" desc:"WaitTime limits how long a watches for Consul resources will block. If not provided, the agent default values will be used."`
	ServiceDiscovery   *ServiceDiscoveryOptions `json:"serviceDiscovery,omitEmpty" desc:"Enable Service Discovery via Consul with this field set to empty struct '{}' to enable with defaults"`
	HttpAddress        *string                  `json:"httpAddress,omitEmpty" desc:"The address of the Consul HTTP server. Used by service discovery and key-value storage (if-enabled). Defaults to the value of the standard CONSUL_HTTP_ADDR env if set, otherwise to 127.0.0.1:8500."`
	DnsAddress         *string                  `json:"dnsAddress,omitEmpty" desc:"The address of the DNS server used to resolve hostnames in the Consul service address. Used by service discovery (required when Consul service instances are stored as DNS names). Defaults to 127.0.0.1:8600. (the default Consul DNS server)"`
	DnsPollingInterval *Duration                `json:"dnsPollingInterval,omitEmpty" desc:"The polling interval for the DNS server. If there is a Consul service address with a hostname instead of an IP, Gloo Edge will resolve the hostname with the configured frequency to update endpoints with any changes to DNS resolution. Defaults to 5s."`
}

type ServiceDiscoveryOptions struct {
	DataCenters []string `json:"dataCenters,omitEmpty" desc:"Use this parameter to restrict the data centers that will be considered when discovering and routing to services. If not provided, Gloo Edge will use all available data centers."`
}

type ConsulUpstreamDiscovery struct {
	UseTlsDiscovery  *bool        `json:"useTlsDiscovery,omitEmpty" desc:"Allow Gloo Edge to automatically apply tls to consul services that are tagged the tlsTagName value. Requires RootCaResourceNamespace and RootCaResourceName to be set if true."`
	TlsTagName       *string      `json:"tlsTagName,omitEmpty" desc:"The tag Gloo Edge should use to identify consul services that ought to use TLS. If splitTlsServices is true, then this tag is also used to sort serviceInstances into the tls upstream. Defaults to 'glooUseTls'."`
	SplitTlsServices *bool        `json:"splitTlsServices,omitEmpty" desc:"If true, then create two upstreams to be created when a consul service contains the tls tag; one with TLS and one without."`
	DiscoveryRootCa  *ResourceRef `json:"discoveryRootCa,omitempty" desc:"The name/namespace of the root CA needed to use TLS with consul services."`
}

// equivalent of core.solo.io.ResourceRef
type ResourceRef struct {
	Namespace *string `json:"namespace,omitEmpty" desc:"The namespace of this resource."`
	Name      *string `json:"name,omitEmpty" desc:"The name of this resource."`
}

// google.protobuf.Duration
type Duration struct {
	Seconds *int32 `json:"seconds,omitEmpty" desc:"The value of this duration in seconds."`
	Nanos   *int32 `json:"nanos,omitEmpty" desc:"The value of this duration in nanoseconds."`
}

type Knative struct {
	Enabled                    *bool             `json:"enabled,omitempty" desc:"enabled knative components"`
	Version                    *string           `json:"version,omitEmpty" desc:"the version of knative installed to the cluster. if using version < 0.8.0, Gloo Edge will use Knative's ClusterIngress API for configuration rather than the namespace-scoped Ingress"`
	Proxy                      *KnativeProxy     `json:"proxy,omitempty"`
	RequireIngressClass        *bool             `json:"requireIngressClass,omitempty" desc:"only serve traffic for Knative Ingress objects with the annotation 'networking.knative.dev/ingress.class: gloo.ingress.networking.knative.dev'."`
	ExtraKnativeInternalLabels map[string]string `json:"extraKnativeInternalLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the knative internal deployment."`
	ExtraKnativeExternalLabels map[string]string `json:"extraKnativeExternalLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the knative external deployment."`
}

type KnativeProxy struct {
	Image                          *Image                `json:"image,omitempty"`
	HttpPort                       *int                  `json:"httpPort,omitempty" desc:"HTTP port for the proxy"`
	HttpsPort                      *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the proxy"`
	Tracing                        *string               `json:"tracing,omitempty" desc:"tracing configuration"`
	LoopBackAddress                *string               `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	ExtraClusterIngressProxyLabels map[string]string     `json:"extraClusterIngressProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the cluster ingress proxy deployment."`
	Internal                       *KnativeProxyInternal `json:"internal,omitempty" desc:"kube resource overrides for knative internal proxy resources"`
	*DeploymentSpec
	*ServiceSpec
	ConfigMap *KubeResourceOverride `json:"configMap,omitempty"`
}

type KnativeProxyInternal struct {
	Deployment *KubeResourceOverride `json:"deployment,omitempty"`
	Service    *KubeResourceOverride `json:"service,omitempty"`
	ConfigMap  *KubeResourceOverride `json:"configMap,omitempty"`
}

type Settings struct {
	WatchNamespaces               []string             `json:"watchNamespaces,omitempty" desc:"whitelist of namespaces for Gloo Edge to watch for services and CRDs. Empty list means all namespaces"`
	WriteNamespace                *string              `json:"writeNamespace,omitempty" desc:"namespace where intermediary CRDs will be written to, e.g. Upstreams written by Gloo Edge Discovery."`
	Integrations                  *Integrations        `json:"integrations,omitempty"`
	Create                        *bool                `json:"create,omitempty" desc:"create a Settings CRD which provides bootstrap configuration to Gloo Edge controllers"`
	Extensions                    interface{}          `json:"extensions,omitempty"`
	SingleNamespace               *bool                `json:"singleNamespace,omitempty" desc:"Enable to use install namespace as WatchNamespace and WriteNamespace"`
	InvalidConfigPolicy           *InvalidConfigPolicy `json:"invalidConfigPolicy,omitempty" desc:"Define policies for Gloo Edge to handle invalid configuration"`
	Linkerd                       *bool                `json:"linkerd,omitempty" desc:"Enable automatic Linkerd integration in Gloo Edge"`
	DisableProxyGarbageCollection *bool                `json:"disableProxyGarbageCollection,omitempty" desc:"Set this option to determine the state of an Envoy listener when the corresponding Proxy resource has no routes. If false (default), Gloo Edge will propagate the state of the Proxy to Envoy, resetting the listener to a clean slate with no routes. If true, Gloo Edge will keep serving the routes from the last applied valid configuration."`
	RegexMaxProgramSize           *uint32              `json:"regexMaxProgramSize,omitempty" desc:"Set this field to specify the RE2 default max program size which is a rough estimate of how complex the compiled regex is to evaluate. If not specified, this defaults to 100."`
	DisableKubernetesDestinations *bool                `json:"disableKubernetesDestinations,omitempty" desc:"Gloo Edge allows you to directly reference a Kubernetes service as a routing destination. To enable this feature, Gloo Edge scans the cluster for Kubernetes services and creates a special type of in-memory Upstream to represent them. If the cluster contains a lot of services and you do not restrict the namespaces Gloo Edge is watching, this can result in significant overhead. If you do not plan on using this feature, you can set this flag to true to turn it off."`
	Aws                           AwsSettings          `json:"aws,omitempty"`
	RateLimit                     interface{}          `json:"rateLimit,omitempty" desc:"Partial config for Gloo Edge Enterprise’s rate-limiting service, based on Envoy’s rate-limit service; supports Envoy’s rate-limit service API. (reference here: https://github.com/lyft/ratelimit#configuration) Configure rate-limit descriptors here, which define the limits for requests based on their descriptors. Configure rate-limits (composed of actions, which define how request characteristics get translated into descriptors) on the VirtualHost or its routes."`
	EnableRestEds                 *bool                `json:"enableRestEds,omitempty" desc:"Whether or not to use rest xds for all EDS by default. Defaults to false."`
	*KubeResourceOverride
}

type AwsSettings struct {
	EnableCredentialsDiscovery      *bool   `json:"enableCredentialsDiscovery,omitempty" desc:"Enable AWS credentials discovery in Envoy for lambda requests. If enableServiceAccountCredentials is also set, it will take precedence as only one may be enabled in Gloo Edge"`
	EnableServiceAccountCredentials *bool   `json:"enableServiceAccountCredentials,omitempty" desc:"Use ServiceAccount credentials to authenticate lambda requests. If enableCredentialsDiscovery is also set, this will take precedence as only one may be enabled in Gloo Edge"`
	StsCredentialsRegion            *string `json:"stsCredentialsRegion,omitempty" desc:"Regional endpoint to use for AWS STS requests. If empty will default to global sts endpoint."`
}

type InvalidConfigPolicy struct {
	ReplaceInvalidRoutes     *bool   `json:"replaceInvalidRoutes,omitempty" desc:"Rather than pausing configuration updates, in the event of an invalid Route defined on a virtual service or route table, Gloo Edge will serve the route with a predefined direct response action. This allows valid routes to be updated when other routes are invalid."`
	InvalidRouteResponseCode *int64  `json:"invalidRouteResponseCode,omitempty" desc:"the response code for the direct response"`
	InvalidRouteResponseBody *string `json:"invalidRouteResponseBody,omitempty" desc:"the response body for the direct response"`
}

type Gloo struct {
	Deployment     *GlooDeployment       `json:"deployment,omitempty"`
	GlooService    *KubeResourceOverride `json:"service,omitempty"`
	ServiceAccount `json:"serviceAccount,omitempty" `
	LogLevel       *string `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
}

type GlooDeployment struct {
	Image                  *Image            `json:"image,omitempty"`
	XdsPort                *int              `json:"xdsPort,omitempty" desc:"port where gloo serves xDS API to Envoy"`
	RestXdsPort            *uint32           `json:"restXdsPort,omitempty" desc:"port where gloo serves REST xDS API to Envoy"`
	ValidationPort         *int              `json:"validationPort,omitempty" desc:"port where gloo serves gRPC Proxy Validation to Gateway"`
	Stats                  *Stats            `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gloo pod"`
	FloatingUserId         *bool             `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser              *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	ExternalTrafficPolicy  *string           `json:"externalTrafficPolicy,omitempty" desc:"Set the external traffic policy on the gloo service"`
	DisableUsageStatistics *bool             `json:"disableUsageStatistics,omitempty" desc:"Disable the collection of gloo usage statistics"`
	ExtraGlooLabels        map[string]string `json:"extraGlooLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the primary gloo deployment."`
	*DeploymentSpec
}

type Discovery struct {
	Deployment     *DiscoveryDeployment `json:"deployment,omitempty"`
	FdsMode        *string              `json:"fdsMode,omitempty" desc:"mode for function discovery (blacklist or whitelist). See more info in the settings docs"`
	Enabled        *bool                `json:"enabled,omitempty" desc:"enable Discovery features"`
	ServiceAccount `json:"serviceAccount,omitempty" `
	LogLevel       *string `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
}

type DiscoveryDeployment struct {
	Image                    Image             `json:"image,omitempty"`
	Stats                    Stats             `json:"stats,omitempty" desc:"overrides for prometheus stats published by the discovery pod"`
	FloatingUserId           *bool             `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser                *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	FsGroup                  *float64          `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
	ExtraDiscoveryLabels     map[string]string `json:"extraDiscoveryLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the gloo edge discovery deployment."`
	EnablePodSecurityContext *bool             `json:"enablePodSecurityContext,omitempty" desc:"Whether or not to render the pod security context. Default is true"`
	*DeploymentSpec
}

type Gateway struct {
	Enabled                       *bool              `json:"enabled,omitempty" desc:"enable Gloo Edge API Gateway features"`
	Validation                    GatewayValidation  `json:"validation,omitempty" desc:"enable Validation Webhook on the Gateway. This will cause requests to modify Gateway-related Custom Resources to be validated by the Gateway."`
	Deployment                    *GatewayDeployment `json:"deployment,omitempty"`
	CertGenJob                    *CertGenJob        `json:"certGenJob,omitempty" desc:"generate self-signed certs with this job to be used with the gateway validation webhook. this job will only run if validation is enabled for the gateway"`
	UpdateValues                  *bool              `json:"updateValues,omitempty" desc:"if true, will use a provided helm helper 'gloo.updatevalues' to update values during template render - useful for plugins/extensions"`
	ProxyServiceAccount           ServiceAccount     `json:"proxyServiceAccount,omitempty" `
	ServiceAccount                ServiceAccount     `json:"serviceAccount,omitempty" `
	ReadGatewaysFromAllNamespaces *bool              `json:"readGatewaysFromAllNamespaces,omitempty" desc:"if true, read Gateway custom resources from all watched namespaces rather than just the namespace of the Gateway controller"`
	CompressedProxySpec           *bool              `json:"compressedProxySpec,omitempty" desc:"if true, enables compression for the Proxy CRD spec"`
	LogLevel                      *string            `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
	GatewayService                *KubeResourceOverride
}

type ServiceAccount struct {
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the service account"`
	DisableAutomount *bool             `json:"disableAutomount,omitempty" desc:"disable automunting the service account to the gateway proxy. not mounting the token hardens the proxy container, but may interfere with service mesh integrations"`
	*KubeResourceOverride
}

type GatewayValidation struct {
	Enabled                         *bool    `json:"enabled,omitempty" desc:"enable Gloo Edge API Gateway validation hook (default true)"`
	AlwaysAcceptResources           *bool    `json:"alwaysAcceptResources,omitempty" desc:"unless this is set this to false in order to ensure validation webhook rejects invalid resources. by default, validation webhook will only log and report metrics for invalid resource admission without rejecting them outright."`
	AllowWarnings                   *bool    `json:"allowWarnings,omitempty" desc:"set this to false in order to ensure validation webhook rejects resources that would have warning status or rejected status, rather than just rejected."`
	DisableTransformationValidation *bool    `json:"disableTransformationValidation,omitempty" desc:"set this to true to disable transformation validation. This may bring signifigant performance benefits if using many transformations, at the cost of possibly incorrect transformations being sent to envoy. When using this value make sure to pre-validate transformations."`
	WarnRouteShortCircuiting        *bool    `json:"warnRouteShortCircuiting,omitempty" desc:"Write a warning to route resources if validation produced a route ordering warning (defaults to false). By setting to true, this means that Gloo Edge will start assigning warnings to resources that would result in route short-circuiting within a virtual host."`
	SecretName                      *string  `json:"secretName,omitempty" desc:"Name of the Kubernetes Secret containing TLS certificates used by the validation webhook server. This secret will be created by the certGen Job if the certGen Job is enabled."`
	FailurePolicy                   *string  `json:"failurePolicy,omitempty" desc:"failurePolicy defines how unrecognized errors from the Gateway validation endpoint are handled - allowed values are 'Ignore' or 'Fail'. Defaults to Ignore "`
	Webhook                         *Webhook `json:"webhook,omitempty" desc:"webhook specific configuration"`
}

type Webhook struct {
	Enabled          *bool             `json:"enabled,omitempty" desc:"enable validation webhook (default true)"`
	DisableHelmHook  *bool             `json:"disableHelmHook,omitempty" desc:"do not create the webhook as helm hook (default false)"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the webhook"`
	*KubeResourceOverride
}

type GatewayDeployment struct {
	Image              *Image            `json:"image,omitempty,omitempty"`
	Stats              *Stats            `json:"stats,omitempty,omitempty" desc:"overrides for prometheus stats published by the gateway pod"`
	FloatingUserId     *bool             `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser          *float64          `json:"runAsUser, omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	ExtraGatewayLabels map[string]string `json:"extraGatewayLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the gloo edge gateway deployment."`
	*DeploymentSpec
}

type Job struct {
	Image *Image `json:"image,omitempty"`
	*JobSpec
	KubeResourceOverride     map[string]interface{} `json:"kubeResourceOverride,omitempty" desc:"override fields in the gateway-certgen job."`
	MtlsKubeResourceOverride map[string]interface{} `json:"mtlsKubeResourceOverride,omitempty" desc:"override fields in the gloo-mtls-certgen job."`
}

type CertGenJob struct {
	Job
	Enabled                 *bool                 `json:"enabled,omitempty" desc:"enable the job that generates the certificates for the validating webhook at install time (default true)"`
	SetTtlAfterFinished     *bool                 `json:"setTtlAfterFinished,omitempty" desc:"Set ttlSecondsAfterFinished (a k8s feature in Alpha) on the job. Defaults to true"`
	TtlSecondsAfterFinished *int                  `json:"ttlSecondsAfterFinished,omitempty" desc:"Clean up the finished job after this many seconds. Defaults to 60"`
	FloatingUserId          *bool                 `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser               *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	Resources               *ResourceRequirements `json:"resources,omitempty"`
}

type GatewayProxy struct {
	Kind                           *GatewayProxyKind            `json:"kind,omitempty" desc:"value to determine how the gateway proxy is deployed"`
	PodTemplate                    *GatewayProxyPodTemplate     `json:"podTemplate,omitempty"`
	ConfigMap                      *ConfigMap                   `json:"configMap,omitempty"`
	CustomStaticLayer              interface{}                  `json:"customStaticLayer,omitempty" desc:"static layer configuration (global overrides for envoy behavior) defined in envoy bootstrap yaml"`
	GlobalDownstreamMaxConnections *uint32                      `json:"globalDownstreamMaxConnections,omitempty" desc:"the number of concurrent connections needed. limit used to protect against exhausting file descriptors on host machine"`
	HealthyPanicThreshold          *int8                        `json:"healthyPanicThreshold,omitempty" desc:"the percentage of healthy hosts required to load balance based on health status of hosts"`
	Service                        *GatewayProxyService         `json:"service,omitempty"`
	AntiAffinity                   *bool                        `json:"antiAffinity,omitempty" desc:"configure anti affinity such that pods are preferably not co-located"`
	Affinity                       []map[string]interface{}     `json:"affinity,omitempty"`
	Tracing                        *Tracing                     `json:"tracing,omitempty"`
	GatewaySettings                *GatewayProxyGatewaySettings `json:"gatewaySettings,omitempty" desc:"settings for the helm generated gateways, leave nil to not render"`
	ExtraEnvoyArgs                 []string                     `json:"extraEnvoyArgs,omitempty" desc:"envoy container args, (e.g. https://www.envoyproxy.io/docs/envoy/latest/operations/cli)"`
	ExtraContainersHelper          *string                      `json:"extraContainersHelper,omitempty"`
	ExtraInitContainersHelper      *string                      `json:"extraInitContainersHelper,omitempty"`
	ExtraVolumes                   []map[string]interface{}     `json:"extraVolumes,omitempty"`
	ExtraVolumeHelper              *string                      `json:"extraVolumeHelper,omitempty"`
	ExtraListenersHelper           *string                      `json:"extraListenersHelper,omitempty"`
	Stats                          *Stats                       `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gateway-proxy pod"`
	ReadConfig                     *bool                        `json:"readConfig,omitempty" desc:"expose a read-only subset of the envoy admin api"`
	ReadConfigMulticluster         *bool                        `json:"readConfigMulticluster,omitempty" desc:"expose a read-only subset of the envoy admin api to gloo-fed"`
	ExtraProxyVolumeMounts         []map[string]interface{}     `json:"extraProxyVolumeMounts,omitempty"`
	ExtraProxyVolumeMountHelper    *string                      `json:"extraProxyVolumeMountHelper,omitempty" desc:"name of custom made named template allowing for extra volume mounts on the proxy container"`
	LoopBackAddress                *string                      `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	Failover                       Failover                     `json:"failover,omitempty" desc:"(Enterprise Only): Failover configuration"`
	Disabled                       *bool                        `json:"disabled,omitempty" desc:"Skips creation of this gateway proxy. Used to turn off gateway proxies created by preceding configurations"`
	EnvoyApiVersion                *string                      `json:"envoyApiVersion,omitempty" desc:"Version of the envoy API to use for the xDS transport and resources. Default is V3"`
	EnvoyBootstrapExtensions       []map[string]interface{}     `json:"envoyBootstrapExtensions,omitempty" desc:"List of bootstrap extensions to add to envoy bootstrap config. Examples include Wasm Service (https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/wasm/v3/wasm.proto#extensions-wasm-v3-wasmservice)."`
	EnvoyStaticClusters            []map[string]interface{}     `json:"envoyStaticClusters,omitempty" desc:"List of extra static clusters to be added to envoy bootstrap config. https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#envoy-v3-api-msg-config-cluster-v3-cluster"`
	HorizontalPodAutoscaler        *HorizontalPodAutoscaler     `json:"horizontalPodAutoscaler,omitempty" desc:"HorizontalPodAutoscaler for the GatewayProxy. Used only when Kind is set to Deployment. Resources must be set on the gateway-proxy deployment for HorizontalPodAutoscalers to function correctly"`
	PodDisruptionBudget            *PodDisruptionBudget         `json:"podDisruptionBudget,omitempty" desc:"PodDisruptionBudget is an object to define the max disruption that can be caused to the gate-proxy pods"`
	IstioMetaMeshId                *string                      `json:"istioMetaMeshId,omitempty" desc:"ISTIO_META_MESH_ID Environment Variable. Defaults to \"cluster.local\""`
	IstioMetaClusterId             *string                      `json:"istioMetaClusterId,omitempty" desc:"ISTIO_META_CLUSTER_ID Environment Variable. Defaults to \"Kubernetes\""`
	LogLevel                       *string                      `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
	*KubeResourceOverride
}

type GatewayProxyGatewaySettings struct {
	DisableGeneratedGateways *bool                    `json:"disableGeneratedGateways,omitempty" desc:"set to true to disable the gateway generation for a gateway proxy"`
	DisableHttpGateway       *bool                    `json:"disableHttpGateway,omitempty" desc:"Set to true to disable http gateway generation."`
	DisableHttpsGateway      *bool                    `json:"disableHttpsGateway,omitempty" desc:"Set to true to disable https gateway generation."`
	IPv4Only                 *bool                    `json:"ipv4Only,omitempty" desc:"set to true if your network allows ipv4 addresses only. Sets the Gateway spec's bindAddress to 0.0.0.0 instead of ::"`
	UseProxyProto            *bool                    `json:"useProxyProto,omitempty" desc:"use proxy protocol"`
	CustomHttpGateway        *string                  `json:"customHttpGateway,omitempty" desc:"custom yaml to use for http gateway settings"`
	CustomHttpsGateway       *string                  `json:"customHttpsGateway,omitempty" desc:"custom yaml to use for https gateway settings"`
	AccessLoggingService     als.AccessLoggingService `json:"accessLoggingService,omitempty"`
	GatewayOptions           v1.ListenerOptions       `json:"options,omitempty" desc:"custom options for http(s) gateways"`
	HttpGatewayOverride      *KubeResourceOverride    `json:"httpGatewayKubeOverride,omitempty"`
	HttpsGatewayOverride     *KubeResourceOverride    `json:"httpsGatewayKubeOverride,omitempty"`
	*KubeResourceOverride
}

type GatewayProxyKind struct {
	Deployment *GatewayProxyDeployment `json:"deployment,omitempty" desc:"set to deploy as a kubernetes deployment, otherwise nil"`
	DaemonSet  *DaemonSetSpec          `json:"daemonSet,omitempty" desc:"set to deploy as a kubernetes daemonset, otherwise nil"`
}
type GatewayProxyDeployment struct {
	*DeploymentSpecSansResources
	*KubeResourceOverride
}

type HorizontalPodAutoscaler struct {
	ApiVersion                     *string                  `json:"apiVersion,omitempty" desc:"accepts autoscaling/v1 or autoscaling/v2beta2."`
	MinReplicas                    *int32                   `json:"minReplicas,omitempty" desc:"minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down."`
	MaxReplicas                    *int32                   `json:"maxReplicas,omitempty" desc:"maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up. It cannot be less that minReplicas."`
	TargetCPUUtilizationPercentage *int32                   `json:"targetCPUUtilizationPercentage,omitempty" desc:"target average CPU utilization (represented as a percentage of requested CPU) over all the pods. Used only with apiVersion autoscaling/v1"`
	Metrics                        []map[string]interface{} `json:"metrics,omitempty" desc:"metrics contains the specifications for which to use to calculate the desired replica count (the maximum replica count across all metrics will be used). Used only with apiVersion autoscaling/v2beta2"`
	Behavior                       map[string]interface{}   `json:"behavior,omitempty" desc:"behavior configures the scaling behavior of the target in both Up and Down directions (scaleUp and scaleDown fields respectively). Used only with apiVersion autoscaling/v2beta2"`
	*KubeResourceOverride
}

type PodDisruptionBudget struct {
	MinAvailable   *int32 `json:"minAvailable,omitempty" desc:"An eviction is allowed if at least \"minAvailable\" pods selected by \"selector\" will still be available after the eviction, i.e. even in the absence of the evicted pod. So for example you can prevent all voluntary evictions by specifying \"100%\"."`
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty" desc:"An eviction is allowed if at most \"maxUnavailable\" pods selected by \"selector\" are unavailable after the eviction, i.e. even in absence of the evicted pod. For example, one can prevent all voluntary evictions by specifying 0. This is a mutually exclusive setting with \"minAvailable\"."`
	*KubeResourceOverride
}

type DaemonSetSpec struct {
	HostPort    *bool `json:"hostPort,omitempty" desc:"whether or not to enable host networking on the pod. Only relevant when running as a DaemonSet"`
	HostNetwork *bool `json:"hostNetwork,omitempty"`
}

type GatewayProxyPodTemplate struct {
	Image                         *Image                `json:"image,omitempty"`
	HttpPort                      *int                  `json:"httpPort,omitempty" desc:"HTTP port for the gateway service target port"`
	HttpsPort                     *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service target port"`
	ExtraPorts                    []interface{}         `json:"extraPorts,omitempty" desc:"extra ports for the gateway pod"`
	ExtraAnnotations              map[string]string     `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the pod"`
	NodeName                      *string               `json:"nodeName,omitempty" desc:"name of node to run on"`
	NodeSelector                  map[string]string     `json:"nodeSelector,omitempty" desc:"label selector for nodes"`
	Tolerations                   []*appsv1.Toleration  `json:"tolerations,omitEmpty"`
	Probes                        *bool                 `json:"probes,omitempty" desc:"enable liveness and readiness probes"`
	Resources                     *ResourceRequirements `json:"resources,omitempty"`
	DisableNetBind                *bool                 `json:"disableNetBind,omitempty" desc:"don't add the NET_BIND_SERVICE capability to the pod. This means that the gateway proxy will not be able to bind to ports below 1024"`
	RunUnprivileged               *bool                 `json:"runUnprivileged,omitempty" desc:"run envoy as an unprivileged user"`
	FloatingUserId                *bool                 `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser                     *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	FsGroup                       *float64              `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
	GracefulShutdown              *GracefulShutdownSpec `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int                  `json:"terminationGracePeriodSeconds,omitempty" desc:"Time in seconds to wait for the pod to terminate gracefully. See [kubernetes docs](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core) for more info"`
	CustomReadinessProbe          *appsv1.Probe         `json:"customReadinessProbe,omitEmpty"`
	ExtraGatewayProxyLabels       map[string]string     `json:"extraGatewayProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the gloo edge gateway-proxy deployment."`
	EnablePodSecurityContext      *bool                 `json:"enablePodSecurityContext,omitempty" desc:"Whether or not to render the pod security context. Default is true"`
}

type GracefulShutdownSpec struct {
	Enabled          *bool `json:"enabled,omitempty" desc:"Enable grace period before shutdown to finish current requests while envoy health checks fail to e.g. notify external load balancers. *NOTE:* This will not have any effect if you have not defined health checks via the health check filter"`
	SleepTimeSeconds *int  `json:"sleepTimeSeconds,omitempty" desc:"Time (in seconds) for the preStop hook to wait before allowing envoy to terminate"`
}

type GatewayProxyService struct {
	Type                     *string               "json:\"type,omitempty\" desc:\"gateway [service type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types). default is `LoadBalancer`\""
	HttpPort                 *int                  `json:"httpPort,omitempty" desc:"HTTP port for the gateway service"`
	HttpsPort                *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service"`
	HttpNodePort             *int                  `json:"httpNodePort,omitempty" desc:"HTTP nodeport for the gateway service if using type NodePort"`
	HttpsNodePort            *int                  `json:"httpsNodePort,omitempty" desc:"HTTPS nodeport for the gateway service if using type NodePort"`
	ClusterIP                *string               "json:\"clusterIP,omitempty\" desc:\"static clusterIP (or `None`) when `gatewayProxies[].gatewayProxy.service.type` is `ClusterIP`\""
	ExtraAnnotations         map[string]string     `json:"extraAnnotations,omitempty"`
	ExternalTrafficPolicy    *string               `json:"externalTrafficPolicy,omitempty"`
	Name                     *string               `json:"name,omitempty" desc:"Custom name override for the service resource of the proxy"`
	HttpsFirst               *bool                 `json:"httpsFirst,omitempty" desc:"List HTTPS port before HTTP"`
	LoadBalancerIP           *string               `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
	LoadBalancerSourceRanges []string              `json:"loadBalancerSourceRanges,omitempty" desc:"List of IP CIDR ranges that are allowed to access the load balancer"`
	CustomPorts              []interface{}         `json:"customPorts,omitempty" desc:"List of custom port to expose in the envoy proxy. Each element follows conventional port syntax (port, targetPort, protocol, name)"`
	ExternalIPs              []string              `json:"externalIPs,omitempty" desc:"externalIPs is a list of IP addresses for which nodes in the cluster will also accept traffic for this service"`
	ConfigDumpService        *KubeResourceOverride `json:"configDumpService,omitempty" desc:"kube resource override for gateway proxy config dump service"`
	*KubeResourceOverride
}

type Tracing struct {
	Provider *string `json:"provider,omitempty"`
	Cluster  *string `json:"cluster,omitempty"`
}

type Failover struct {
	Enabled    *bool   `json:"enabled,omitempty" desc:"(Enterprise Only): Configure this proxy for failover"`
	Port       *uint   `json:"port,omitempty" desc:"(Enterprise Only): Port to use for failover Gateway Bind port, and service. Default is 15443"`
	NodePort   *uint   `json:"nodePort,omitempty" desc:"(Enterprise Only): Optional NodePort for failover Service"`
	SecretName *string `json:"secretName,omitempty" desc:"(Enterprise Only): Secret containing downstream Ssl Secrets Default is failover-downstream"`
	*KubeResourceOverride
}

type AccessLogger struct {
	Image                   *Image                `json:"image,omitempty"`
	Port                    *uint                 `json:"port,omitempty"`
	ServiceName             *string               `json:"serviceName,omitempty"`
	Enabled                 *bool                 `json:"enabled,omitempty"`
	Stats                   *Stats                `json:"stats,omitempty" desc:"overrides for prometheus stats published by the access logging pod"`
	RunAsUser               *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	FsGroup                 *float64              `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
	ExtraAccessLoggerLabels map[string]string     `json:"extraAccessLoggerLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the access logger deployment."`
	Service                 *KubeResourceOverride `json:"service,omitempty"`
	*DeploymentSpec
}

type GatewayProxyConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
}

type Ingress struct {
	Enabled             *bool              `json:"enabled,omitempty"`
	Deployment          *IngressDeployment `json:"deployment,omitempty"`
	RequireIngressClass *bool              `json:"requireIngressClass,omitempty" desc:"only serve traffic for Ingress objects with the Ingress Class annotation 'kubernetes.io/ingress.class'. By default the annotation value must be set to 'gloo', however this can be overriden via customIngressClass."`
	CustomIngress       *bool              `json:"customIngressClass,omitempty" desc:"Only relevant when requireIngressClass is set to true. Setting this value will cause the Gloo Edge Ingress Controller to process only those Ingress objects which have their ingress class set to this value (e.g. 'kubernetes.io/ingress.class=SOMEVALUE')."`
}

type IngressDeployment struct {
	Image              *Image            `json:"image,omitempty"`
	RunAsUser          *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	FloatingUserId     *bool             `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	ExtraIngressLabels map[string]string `json:"extraIngressLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the ingress deployment."`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment      *IngressProxyDeployment `json:"deployment,omitempty"`
	ConfigMap       *ConfigMap              `json:"configMap,omitempty"`
	Tracing         *string                 `json:"tracing,omitempty"`
	LoopBackAddress *string                 `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	Label           *string                 `json:"label,omitempty" desc:"Value for label gloo. Use a unique value to use several ingress proxy instances in the same cluster. Default is ingress-proxy"`
	*ServiceSpec
}

type IngressProxyDeployment struct {
	Image                   *Image            `json:"image,omitempty"`
	HttpPort                *int              `json:"httpPort,omitempty" desc:"HTTP port for the ingress container"`
	HttpsPort               *int              `json:"httpsPort,omitempty" desc:"HTTPS port for the ingress container"`
	ExtraPorts              []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations        map[string]string `json:"extraAnnotations,omitempty"`
	FloatingUserId          *bool             `json:"floatingUserId,omitempty" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser               *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the pod to run as. Default is 10101"`
	ExtraIngressProxyLabels map[string]string `json:"extraIngressProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the ingress proxy deployment."`
	*DeploymentSpec
}

type ServiceSpec struct {
	Service *Service `json:"service,omitempty" desc:"K8s service configuration"`
}

type Service struct {
	Type             *string           `json:"type,omitempty" desc:"K8s service type"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the service"`
	LoadBalancerIP   *string           `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
	HttpPort         *int              `json:"httpPort,omitempty" desc:"HTTP port for the knative/ingress proxy service"`
	HttpsPort        *int              `json:"httpsPort,omitempty" desc:"HTTPS port for the knative/ingress proxy service"`
	*KubeResourceOverride
}

type ConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
	*KubeResourceOverride
}

type K8s struct {
	ClusterName *string `json:"clusterName,omitempty" desc:"cluster name to use when referencing services."`
}

type Stats struct {
	Enabled            *bool   `json:"enabled,omitempty" desc:"Controls whether or not envoy stats are enabled"`
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty" desc:"The envoy stats endpoint to which the metrics are written"`
}

type Mtls struct {
	Enabled               *bool                 `json:"enabled,omitempty" desc:"Enables internal mtls authentication"`
	Sds                   SdsContainer          `json:"sds,omitempty"`
	EnvoySidecar          EnvoySidecarContainer `json:"envoy,omitempty"`
	EnvoySidecarResources *ResourceRequirements `json:"envoySidecarResources,omitempty" desc:"Sets default resource requirements for all envoy sidecar containers."`
	SdsResources          *ResourceRequirements `json:"sdsResources,omitempty" desc:"Sets default resource requirements for all sds containers."`
}

type SdsContainer struct {
	Image *Image `json:"image,omitempty"`
}

type EnvoySidecarContainer struct {
	Image *Image `json:"image,omitempty"`
}

type IstioSDS struct {
	Enabled        *bool         `json:"enabled,omitempty" desc:"Enables SDS cert-rotator sidecar for istio mTLS cert rotation"`
	CustomSidecars []interface{} `json:"customSidecars,omitempty" desc:"Override the default Istio sidecar in gateway-proxy with a custom container. Ignored if IstioSDS.enabled is false"`
}

type IstioIntegration struct {
	LabelInstallNamespace *bool `json:"labelInstallNamespace,omitempty" desc:"If creating a namespace for Gloo, include the 'istio-injection: enabled' label to allow Istio sidecar injection for Gloo pods. Be aware that Istio's default injection behavior will auto-inject a sidecar into all pods in such a marked namespace. Disabling this behavior in Istio's configs or using gloo's global.istioIntegration.disableAutoinjection flag is recommended."`
	WhitelistDiscovery    *bool `json:"whitelistDiscovery,omitempty" desc:"Annotate the discovery pod for Istio sidecar injection to ensure that it gets a sidecar even when namespace-wide auto-injection is disabled. Generally only needed for FDS is enabled."`
	DisableAutoinjection  *bool `json:"disableAutoinjection,omitempty" desc:"Annotate all pods (excluding those whitelisted by other config values) to with an explicit 'do not inject' annotation to prevent Istio from adding sidecars to all pods. It's recommended that this be set to true if Gloo's namespace is marked for Istio discovery, as some pods do not immediately work with an Istio sidecar without extra manual configuration."`
}
