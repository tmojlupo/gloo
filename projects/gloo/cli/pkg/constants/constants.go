package constants

const (
	GlooHelmRepoTemplate       = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
	GlooWithUiHelmRepoTemplate = "https://storage.googleapis.com/gloo-os-ui-helm/charts/gloo-os-with-ui-%s.tgz"
	GlooReleaseName            = "gloo"
	GlooFedReleaseName         = "gloo-fed"
	KnativeServingNamespace    = "knative-serving"
)

var (
	// This slice defines the valid prefixes for glooctl extension binaries within the user's PATH (e.g. "glooctl-foo").
	ValidExtensionPrefixes = []string{"glooctl"}
)
