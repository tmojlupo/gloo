package istio

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"

	"github.com/spf13/cobra"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	istioCertSecret        = "istio_server_cert"
	istioValidationContext = "istio_validation_context"
	sdsTargetURI           = "127.0.0.1:8234"
)

// EnableMTLS adds an sslConfig to the given upstream which will
// be used by envoy SDS to pick up the mTLS certs
func EnableMTLS(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable-mtls",
		Short: "Enables Istio mTLS for a given upstream",
		Long:  "Enables Istio mTLS for a given upstream, by adding an sslConfig which lets envoy know to get the certs via SDS",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := istioEnableMTLS(args, opts)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return err
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddUpstreamFlag(pflags, &opts.Istio.Upstream)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func istioEnableMTLS(args []string, opts *options.Options) error {
	upClient := helpers.MustNamespacedUpstreamClient(opts.Metadata.GetNamespace())
	up, err := upClient.Read(opts.Metadata.Namespace, opts.Istio.Upstream, clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading upstream")
	}

	if up.SslConfig != nil {
		return errors.Wrapf(err, "Error upstream already has an sslConfig set")
	}

	up.SslConfig = &gloov1.UpstreamSslConfig{
		AlpnProtocols: []string{"istio"},
		SslSecrets: &gloov1.UpstreamSslConfig_Sds{
			Sds: &gloov1.SDSConfig{
				CertificatesSecretName: istioCertSecret,
				ValidationContextName:  istioValidationContext,
				TargetUri:              sdsTargetURI,
				SdsBuilder: &gloov1.SDSConfig_ClusterName{
					ClusterName: sdsClusterName,
				},
			},
		},
	}

	_, err = upClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
	return err
}
