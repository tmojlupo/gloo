package istio

import (
	"bytes"
	"errors"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/spf13/cobra"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrMissingSidecars occurs when the user tries to uninject istio & sds, but one or both cannot be find.
var ErrMissingSidecars = errors.New("istio uninject can only be run when both the sds and istio-proxy sidecars are present on the gateway-proxy pod")

// List of istio-specific volumes mounted in the gateway-proxy deployment
var istioVolumes = []string{"istio-certs", "istiod-ca-cert", "istio-envoy", "istio-token"}

// Uninject is an istio subcommand in glooctl which can be used to remove a previously
// injected SDS sidecar and an istio-proxy sidecar from the gateway-proxy pod
func Uninject(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninject",
		Short: "Remove SDS & istio-proxy sidecars from gateway-proxy pod",
		Long: "Removes the istio-proxy sidecar from the gateway-proxy pod. " +
			"Also removes the sds sidecar from the gateway-proxy pod. " +
			"Also removes the gateway_proxy_sds cluster from the gateway-proxy envoy bootstrap ConfigMap.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := istioUninject(args, opts)
			if err != nil {
				return err
			}
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

// istioUninject removes SDS & istio-proxy sidecars,
// as well as updating the gateway-proxy ConfigMap
func istioUninject(args []string, opts *options.Options) error {
	glooNS := opts.Metadata.Namespace

	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(glooNS, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Remove gateway_proxy_sds cluster from the gateway-proxy configmap
	configMaps, err := client.CoreV1().ConfigMaps(glooNS).List(metav1.ListOptions{})
	for _, configMap := range configMaps.Items {
		if configMap.Name == gatewayProxyConfigMap {
			// Make sure we don't already have the gateway_proxy_sds cluster set up
			err := removeSdsCluster(&configMap)
			if err != nil {
				return err
			}
			_, err = client.CoreV1().ConfigMaps(glooNS).Update(&configMap)
			if err != nil {
				return err
			}
		}
	}

	deployments, err := client.AppsV1().Deployments(glooNS).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		if deployment.Name == "gateway-proxy" {
			containers := deployment.Spec.Template.Spec.Containers

			// Remove Sidecars
			sdsPresent := false
			istioPresent := false
			if len(containers) > 1 {
				for i := len(containers) - 1; i >= 0; i-- {
					container := containers[i]
					if container.Name == "sds" {
						sdsPresent = true
						copy(containers[i:], containers[i+1:])
						containers = containers[:len(containers)-1]
					}
					if container.Name == "istio-proxy" {
						istioPresent = true

						copy(containers[i:], containers[i+1:])
						containers = containers[:len(containers)-1]
					}
				}
			}

			if !sdsPresent || !istioPresent {
				return ErrMissingSidecars
			}

			deployment.Spec.Template.Spec.Containers = containers

			removeIstioVolumes(&deployment)
			_, err = client.AppsV1().Deployments(glooNS).Update(&deployment)
			if err != nil {
				return err
			}

		}
	}

	return nil
}

// removeIstioVolumes removes the istio volumes from the given deployment,
func removeIstioVolumes(deployment *appsv1.Deployment) {
	volsToRemove := make(map[string]bool)
	for _, v := range istioVolumes {
		volsToRemove[v] = true
	}

	vols := deployment.Spec.Template.Spec.Volumes

	for i := len(vols) - 1; i >= 0; i-- {
		vol := vols[i]
		// If it's in the istioVolumes list, remove it
		_, isIstioVol := volsToRemove[vol.Name]
		if isIstioVol {
			copy(vols[i:], vols[i+1:])
			vols = vols[:len(vols)-1]
		}
	}

	deployment.Spec.Template.Spec.Volumes = vols
}

// removeSdsCluster drops the gateway_proxy_sds cluster from the given ConfigMap
func removeSdsCluster(configMap *corev1.ConfigMap) error {
	old := configMap.Data[envoyDataKey]
	bootstrapConfig, err := envoyConfigFromString(old)
	if err != nil {
		return err
	}

	clusters := bootstrapConfig.StaticResources.Clusters

	for i, cluster := range clusters {
		if cluster.Name == sdsClusterName {
			// Remove the SDS cluster
			copy(clusters[i:], clusters[i+1:])
			clusters = clusters[:len(clusters)-1]
		}
	}

	bootstrapConfig.StaticResources.Clusters = clusters

	// Marshall bootstrapConfig into JSON
	var bootStrapJSON bytes.Buffer
	var marshaller jsonpb.Marshaler
	err = marshaller.Marshal(&bootStrapJSON, &bootstrapConfig)
	if err != nil {
		return err
	}

	// We convert from JSON to YAML rather than marshalling
	// directly from go struct to YAML, because otherwise we
	// end up with a bunch of null values which fail to parse
	yamlConfig, err := yaml.JSONToYAML(bootStrapJSON.Bytes())
	if err != nil {
		return err
	}

	configMap.Data[envoyDataKey] = string(yamlConfig)
	return nil
}
