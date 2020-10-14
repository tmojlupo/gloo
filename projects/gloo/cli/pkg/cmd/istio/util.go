package istio

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List of pods in which we could find the Gloo (OS) version
var glooOSPods = map[string]bool{
	"gateway":   true,
	"ingress":   true,
	"discovery": true,
}

func envoyConfigFromString(config string) (envoy_config_bootstrap.Bootstrap, error) {
	var bootstrapConfig envoy_config_bootstrap.Bootstrap
	bootstrapConfig, err := unmarshalYAMLConfig(config)
	return bootstrapConfig, err
}

func getIstiodContainer(namespace string) (corev1.Container, error) {
	var c corev1.Container
	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return c, err
	}
	deployments, err := client.AppsV1().Deployments(namespace).List(metav1.ListOptions{})
	if err != nil {
		return c, err
	}

	for _, deployment := range deployments.Items {
		if deployment.Name == "istiod" {
			containers := deployment.Spec.Template.Spec.Containers
			for _, container := range containers {
				if container.Name == "discovery" {
					return container, nil
				}
			}

		}
	}
	return c, ErrIstioVerUndetermined

}

// getImageVersion gets the tag from the image of the given container
func getImageVersion(container corev1.Container) (string, error) {
	img := strings.SplitAfter(container.Image, ":")
	if len(img) != 2 {
		return "", ErrImgVerUndetermined
	}
	return img[1], nil
}

// getJWTPolicy gets the JWT policy from istiod
func getJWTPolicy(pilotContainer corev1.Container) string {
	for _, env := range pilotContainer.Env {
		if env.Name == "JWT_POLICY" {
			return env.Value
		}
	}
	// Default to third-party if not found
	fmt.Println("Warning: unable to determine Istio JWT Policy, defaulting to third party")
	return "third-party-jwt"
}

// getGlooVersion gets the version of gloo currently running
// in the given namespace, by checking the gloo deployment.
func getGlooVersion(namespace string) (string, error) {
	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	deployments, err := client.AppsV1().Deployments(namespace).List(metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	// For each deployment
	for _, deployment := range deployments.Items {
		// If it's a Gloo OS pod
		if _, ok := glooOSPods[deployment.Name]; ok {
			containers := deployment.Spec.Template.Spec.Containers
			// Grab the container named the same as deploy (in case of eg istio sidecars)
			for _, container := range containers {
				if container.Name == deployment.Name {
					return getImageVersion(container)
				}
			}
		}
	}
	return "", ErrGlooVerUndetermined
}

// unmarshalYAMLConfig converts from an envoy
// bootstrap yaml into a bootstrapConfig struct
func unmarshalYAMLConfig(configYAML string) (envoy_config_bootstrap.Bootstrap, error) {
	var bootstrapConfig envoy_config_bootstrap.Bootstrap
	// first step - serialize yaml to json
	jsondata, err := yaml.YAMLToJSON([]byte(configYAML))
	if err != nil {
		return bootstrapConfig, err
	}

	// second step - unmarshal from json into a bootstrapConfig object
	jsonreader := bytes.NewReader(jsondata)
	var unmarshaler jsonpb.Unmarshaler
	err = unmarshaler.Unmarshal(jsonreader, &bootstrapConfig)
	return bootstrapConfig, err
}
