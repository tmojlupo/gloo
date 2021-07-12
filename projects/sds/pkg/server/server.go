package server

import (
	"context"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"net"

	"github.com/avast/retry-go"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_transport_sockets_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	cache_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

var (
	grpcOptions = []grpc.ServerOption{grpc.MaxConcurrentStreams(10000)}
)

// Secret represents an envoy auth secret
type Secret struct {
	SslCaFile         string
	SslKeyFile        string
	SslCertFile       string
	ServerCert        string // name of a tls_certificate_sds_secret_config
	ValidationContext string // name of the validation_context_sds_secret_config
}

// Server is the SDS server. Holds config & secrets.
type Server struct {
	secrets       []Secret
	sdsClient     string
	grpcServer    *grpc.Server
	address       string
	snapshotCache cache.SnapshotCache
}

// ID needed for snapshotCache
func (s *Server) ID(_ *envoy_config_core_v3.Node) string {
	return s.sdsClient
}

// SetupEnvoySDS creates a new SDSServer. The returned server can be started with Run()
func SetupEnvoySDS(secrets []Secret, sdsClient, serverAddress string) *Server {
	grpcServer := grpc.NewServer(grpcOptions...)
	sdsServer := &Server{
		secrets:    secrets,
		grpcServer: grpcServer,
		sdsClient:  sdsClient,
		address:    serverAddress,
	}
	snapshotCache := cache.NewSnapshotCache(false, sdsServer, nil)
	sdsServer.snapshotCache = snapshotCache

	svr := server.NewServer(context.Background(), snapshotCache, nil)

	// register services
	envoy_service_secret_v3.RegisterSecretDiscoveryServiceServer(grpcServer, svr)
	return sdsServer
}

// Run starts the server
func (s *Server) Run(ctx context.Context) (<-chan struct{}, error) {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return nil, err
	}
	contextutils.LoggerFrom(ctx).Infof("sds server listening on %s", s.address)
	go func() {
		if err = s.grpcServer.Serve(lis); err != nil {
			contextutils.LoggerFrom(ctx).Fatalw("fatal error in gRPC server", zap.String("address", s.address), zap.Error(err))
		}
	}()
	serverStopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		contextutils.LoggerFrom(ctx).Infof("stopping sds server on %s\n", s.address)
		s.grpcServer.GracefulStop()
		serverStopped <- struct{}{}
	}()
	return serverStopped, nil
}

// UpdateSDSConfig updates with the current certs
func (s *Server) UpdateSDSConfig(ctx context.Context) error {
	var certs [][]byte
	var items []cache_types.Resource
	for _, sec := range s.secrets {
		key, err := readAndVerifyCert(sec.SslKeyFile)
		if err != nil {
			return err
		}
		certs = append(certs, key)
		certChain, err := readAndVerifyCert(sec.SslCertFile)
		if err != nil {
			return err
		}
		certs = append(certs, certChain)
		ca, err := readAndVerifyCert(sec.SslCaFile)
		if err != nil {
			return err
		}
		certs = append(certs, ca)
		items = append(items, serverCertSecret(key, certChain, sec.ServerCert))
		items = append(items, validationContextSecret(ca, sec.ValidationContext))
	}

	snapshotVersion, err := GetSnapshotVersion(certs)
	if err != nil {
		contextutils.LoggerFrom(ctx).Info("Error getting snapshot version", zap.Error(err))
		return err
	}
	contextutils.LoggerFrom(ctx).Infof("Updating SDS config. sdsClient is %s. Snapshot version is %s", s.sdsClient, snapshotVersion)

	secretSnapshot := cache.Snapshot{}
	secretSnapshot.Resources[cache_types.Secret] = cache.NewResources(snapshotVersion, items)
	return s.snapshotCache.SetSnapshot(s.sdsClient, secretSnapshot)
}

// GetSnapshotVersion generates a version string by hashing the certs
func GetSnapshotVersion(certs ...interface{}) (string, error) {
	hash, err := hashutils.HashAllSafe(fnv.New64(), certs...)
	return fmt.Sprintf("%d", hash), err
}

// readAndVerifyCert will read the file from the given
// path, then check for validity every 100ms for 2 seconds.
// This is needed because the filesystem watcher
// that gets triggered by a WRITE doesn't have a guarantee
// that the write has finished yet.
// See https://github.com/fsnotify/fsnotify/pull/252 for more context
func readAndVerifyCert(certFilePath string) ([]byte, error) {
	var err error
	var fileBytes []byte

	var validCerts bool
	// Retry for a few seconds as a write may still be in progress
	err = retry.Do(
		func() error {
			fileBytes, err = ioutil.ReadFile(certFilePath)
			if err != nil {
				return err
			}
			validCerts = checkCert(fileBytes)
			if !validCerts {
				return fmt.Errorf("failed to validate file %v", certFilePath)
			}
			return nil
		},
		retry.Attempts(5), // Exponential backoff over ~3s
	)

	return fileBytes, nil
}

// checkCert uses pem.Decode to verify that the given
// bytes are not malformed, as could be caused by a
// write-in-progress. Uses pem.Decode to check the blocks.
// See https://golang.org/src/encoding/pem/pem.go?s=2505:2553#L76
func checkCert(certs []byte) bool {
	block, rest := pem.Decode(certs)
	if block == nil {
		// Remainder does not contain any certs/keys
		return false
	}
	// Found a cert, check the rest
	if len(rest) > 0 {
		// Something after the cert, validate that too
		return checkCert(rest)
	}
	return true
}

func serverCertSecret(privateKey, certChain []byte, serverCert string) cache_types.Resource {
	return &envoy_extensions_transport_sockets_tls_v3.Secret{
		Name: serverCert,
		Type: &envoy_extensions_transport_sockets_tls_v3.Secret_TlsCertificate{
			TlsCertificate: &envoy_extensions_transport_sockets_tls_v3.TlsCertificate{
				CertificateChain: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
						InlineBytes: certChain,
					},
				},
				PrivateKey: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
						InlineBytes: privateKey,
					},
				},
			},
		},
	}
}

func validationContextSecret(caCert []byte, validationContext string) cache_types.Resource {
	return &envoy_extensions_transport_sockets_tls_v3.Secret{
		Name: validationContext,
		Type: &envoy_extensions_transport_sockets_tls_v3.Secret_ValidationContext{
			ValidationContext: &envoy_extensions_transport_sockets_tls_v3.CertificateValidationContext{
				TrustedCa: &envoy_config_core_v3.DataSource{
					Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
						InlineBytes: caCert,
					},
				},
			},
		},
	}
}
