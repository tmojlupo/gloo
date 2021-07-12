package secret

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
)

func tlsCmd(opts *options.Options) *cobra.Command {
	input := &opts.Create.InputSecret.TlsSecret
	cmd := &cobra.Command{
		Use:   "tls",
		Short: `Create a secret with the given name`,
		Long: "Create a secret with the given name. " +
			"The format of the secret data is: `{\"tls\" : [tls object]}`. " +
			"Note that the annotation `resource_kind: '*v1.Secret'` is added in order for Gloo to find this secret. " +
			"If you're creating a secret through another means, you'll need to add that annotation manually.",
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := TlsSecretArgsInteractive(input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createTlsSecret(opts.Top.Ctx, &opts.Metadata, *input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()

	flags.StringVar(&input.RootCaFilename, "rootca", "", "filename of rootca for secret")
	flags.StringVar(&input.PrivateKeyFilename, "privatekey", "", "filename of privatekey for secret")
	flags.StringVar(&input.CertChainFilename, "certchain", "", "filename of certchain for secret")

	return cmd
}

const (
	tlsPromptRootCa     = "filename of rootca for secret (optional)"
	tlsPromptPrivateKey = "filename of privatekey for secret"
	tlsPromptCertChain  = "filename of certchain for secret"
)

func TlsSecretArgsInteractive(input *options.TlsSecret) error {
	if err := cliutil.GetStringInput("filename of rootca for secret (optional)", &input.RootCaFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("filename of privatekey for secret", &input.PrivateKeyFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("filename of certchain for secret", &input.CertChainFilename); err != nil {
		return err
	}

	return nil
}

func createTlsSecret(ctx context.Context, meta *core.Metadata, input options.TlsSecret, dryRun bool, outputType printers.OutputType) error {

	// read the values

	rootCa, privateKey, certChain, err := input.ReadFiles()
	if err != nil {
		return err
	}

	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Tls{
			Tls: &gloov1.TlsSecret{
				CertChain:  string(certChain),
				PrivateKey: string(privateKey),
				RootCa:     string(rootCa),
			},
		},
	}

	if !dryRun {
		var err error
		secretClient := helpers.MustSecretClient(ctx)
		if secret, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}

	}

	_ = printers.PrintSecrets(gloov1.SecretList{secret}, outputType)
	return nil
}
