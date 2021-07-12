package register

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/skv2/pkg/multicluster/kubeconfig"
	"github.com/solo-io/skv2/pkg/multicluster/register"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Deregister(opts *options.Options) error {
	ctx := context.Background()
	deregisterOpts := opts.Cluster.Deregister
	mgmtKubeCfg, err := kubeconfig.GetClientConfigWithContext("", "", "")
	if err != nil {
		return err
	}
	remoteKubeCfg, err := kubeconfig.GetClientConfigWithContext(deregisterOpts.RemoteKubeConfig, deregisterOpts.RemoteContext, "")
	if err != nil {
		return err
	}

	clusterRegisterOpts := register.RegistrationOptions{
		KubeCfg:          mgmtKubeCfg,
		RemoteKubeCfg:    remoteKubeCfg,
		APIServerAddress: deregisterOpts.LocalClusterDomainOverride,
		ClusterName:      deregisterOpts.ClusterName,
		Namespace:        opts.Cluster.FederationNamespace,
		RemoteNamespace:  deregisterOpts.RemoteNamespace,
		ClusterRoles: []*v1.ClusterRole{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: deregisterOpts.RemoteNamespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationPolicyRules,
			},
		},
		Roles: []*v1.Role{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: deregisterOpts.RemoteNamespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationReadConfigPolicyRules,
			},
		},
	}

	return clusterRegisterOpts.DeregisterCluster(ctx)
}
