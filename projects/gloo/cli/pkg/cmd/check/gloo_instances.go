package check

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/jsonpb"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/solo-apis/pkg/api/fed.solo.io/v1/types"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	glooinstancev1 "github.com/solo-io/solo-apis/pkg/api/fed.solo.io/v1"
)

func CheckMulticlusterResources(opts *options.Options) {
	cfg, err := config.GetConfigWithContext("")
	if err != nil {
		fmt.Printf("Warning: could not get kubernetes config to check multicluster resources: %v. "+
			"Skipping Gloo Instance check.\n", err)
		return
	}
	instanceReader, err := getUnstructuredGlooInstanceReader(cfg)
	if err != nil {
		fmt.Printf("Warning: could not get Gloo Instance client: %v. Skipping Gloo Instance check.\n", err)
		return
	}
	glooInstanceList, err := instanceReader.listGlooInstances(opts.Top.Ctx)
	if err != nil {
		if meta.IsNoMatchError(err) {
			fmt.Print("Skipping Gloo Instance check -- Gloo Federation not detected\n")
			return
		}
		fmt.Printf("Warning: could not list Gloo Instances: %v\n", err)
		return
	}
	glooInstances := glooInstanceList.Items
	if len(glooInstances) < 0 {
		// No Gloo Instance CRD exist, meaning that none are registered.
		return
	}
	fmt.Printf("\nDetected Gloo Federation!\n")
	for _, glooInstance := range glooInstanceList.Items {
		fmt.Printf("\nChecking Gloo Instance %s... ", glooInstance.GetName())
		printGlooInstanceCheckSummary("deployments", glooInstance.Spec.GetCheck().GetDeployments())
		printGlooInstanceCheckSummary("pods", glooInstance.Spec.GetCheck().GetPods())
		printGlooInstanceCheckSummary("settings", glooInstance.Spec.GetCheck().GetSettings())
		printGlooInstanceCheckSummary("upstreams", glooInstance.Spec.GetCheck().GetUpstreams())
		printGlooInstanceCheckSummary("upstream groups", glooInstance.Spec.GetCheck().GetUpstreamGroups())
		printGlooInstanceCheckSummary("auth configs", glooInstance.Spec.GetCheck().GetAuthConfigs())
		printGlooInstanceCheckSummary("virtual services", glooInstance.Spec.GetCheck().GetVirtualServices())
		printGlooInstanceCheckSummary("route tables", glooInstance.Spec.GetCheck().GetRouteTables())
		printGlooInstanceCheckSummary("gateways", glooInstance.Spec.GetCheck().GetGateways())
		printGlooInstanceCheckSummary("proxies", glooInstance.Spec.GetCheck().GetProxies())
		fmt.Printf("\n\n")
	}
}

func printGlooInstanceCheckSummary(resourceType string, resource *types.GlooInstanceSpec_Check_Summary) {
	fmt.Printf("\nChecking %s... ", resourceType)

	ok := true
	for _, errReport := range resource.GetErrors() {
		fmt.Printf("\nFound error in %s %s\n", errReport.GetRef().GetNamespace(), errReport.GetRef().GetName())
		fmt.Printf("Reason: %s\n", errReport.GetMessage())
		ok = false
	}
	for _, warningReport := range resource.GetWarnings() {
		fmt.Printf("Found warning in %s %s\n", warningReport.GetRef().GetNamespace(), warningReport.GetRef().GetName())
		fmt.Printf("Reason: %s\n", warningReport.GetMessage())
		ok = false
	}
	if ok {
		fmt.Printf("OK")
	}
}

// unstructuredGlooInstanceReader provides a forwards-compatible means of listing GlooInstances.
// If new fields are added to the GlooInstance API in future Gloo Fed releases, we do not want to
// fail to list GlooInstances due to an error unmarshaling unrecognized fields.
type unstructuredGlooInstanceReader struct {
	unstructuredReader client.Reader
}

func getUnstructuredGlooInstanceReader(cfg *rest.Config) (*unstructuredGlooInstanceReader, error) {
	scheme := scheme.Scheme
	if err := glooinstancev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	client, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	return &unstructuredGlooInstanceReader{unstructuredReader: client}, nil
}

func (c *unstructuredGlooInstanceReader) listGlooInstances(ctx context.Context) (glooinstancev1.GlooInstanceList, error) {
	glooInstanceGVK := schema.GroupVersionKind{
		Group:   "fed.solo.io",
		Version: "v1",
		Kind:    "GlooInstance",
	}
	unstructuredList := &unstructured.UnstructuredList{}
	unstructuredList.SetGroupVersionKind(glooInstanceGVK)

	if err := c.unstructuredReader.List(ctx, unstructuredList); err != nil {
		return glooinstancev1.GlooInstanceList{}, err
	}

	glooInstanceList := glooinstancev1.GlooInstanceList{}
	for _, item := range unstructuredList.Items {
		glooInstance, err := toGlooInstance(item)
		if err != nil {
			return glooinstancev1.GlooInstanceList{}, err
		}
		glooInstanceList.Items = append(glooInstanceList.Items, glooInstance)
	}

	return glooInstanceList, nil
}

func toGlooInstance(unstr unstructured.Unstructured) (glooinstancev1.GlooInstance, error) {
	unmarshaler := jsonpb.Unmarshaler{AllowUnknownFields: true}

	spec := unstr.Object["spec"]
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return glooinstancev1.GlooInstance{}, err
	}
	specPb := &types.GlooInstanceSpec{}
	if err := unmarshaler.Unmarshal(bytes.NewBuffer(specBytes), specPb); err != nil {
		return glooinstancev1.GlooInstance{}, err
	}

	metadata := unstr.Object["metadata"]
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return glooinstancev1.GlooInstance{}, err
	}
	objectMeta := &v1.ObjectMeta{}
	if err := json.Unmarshal(metadataBytes, objectMeta); err != nil {
		return glooinstancev1.GlooInstance{}, err
	}

	return glooinstancev1.GlooInstance{
		ObjectMeta: *objectMeta,
		Spec:       *specPb,
	}, nil
}
