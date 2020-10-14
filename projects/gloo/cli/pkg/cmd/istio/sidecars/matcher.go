package sidecars

import (
	"errors"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// ErrNoSupportedSidecar occurs when we can't find any premade sidecar for the given Istio version
var ErrNoSupportedSidecar = errors.New("no valid istio sidecar found for this istio version")

// GetIstioSidecar will return an Istio sidecar for the given
// version of Istio, with the given jwtPolicy, to run
// in the gateway-proxy pod
func GetIstioSidecar(istioVersion, jwtPolicy string) (*corev1.Container, error) {
	if strings.HasPrefix(istioVersion, "1.7.") {
		return generateIstio17Sidecar(istioVersion, jwtPolicy), nil
	} else if strings.HasPrefix(istioVersion, "1.6.") {
		return generateIstio16Sidecar(istioVersion, jwtPolicy), nil
	}
	return nil, ErrNoSupportedSidecar
}
