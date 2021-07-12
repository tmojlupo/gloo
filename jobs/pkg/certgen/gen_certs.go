package certgen

import (
	"crypto/x509"
	"fmt"

	"github.com/solo-io/k8s-utils/certutils"
	"k8s.io/client-go/util/cert"
	"knative.dev/pkg/network"
)

func GenCerts(svcName, svcNamespace string) (*certutils.Certificates, error) {
	return certutils.GenerateSelfSignedCertificate(cert.Config{
		CommonName:   fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
		Organization: []string{"solo.io"},
		AltNames: cert.AltNames{
			DNSNames: []string{
				svcName,
				fmt.Sprintf("%s.%s", svcName, svcNamespace),
				fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
				fmt.Sprintf("%s.%s.svc.%s", svcName, svcNamespace, network.GetClusterDomainName()),
			},
		},
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})
}
