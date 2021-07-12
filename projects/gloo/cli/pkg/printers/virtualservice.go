package printers

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func PrintVirtualServices(ctx context.Context, virtualServices v1.VirtualServiceList, outputType OutputType, namespace string) error {
	if outputType == KUBE_YAML {
		return PrintKubeCrdList(virtualServices.AsInputResources(), v1.VirtualServiceCrd)
	}
	return cliutils.PrintList(outputType.String(), "", virtualServices,
		func(data interface{}, w io.Writer) error {
			VirtualServiceTable(ctx, data.(v1.VirtualServiceList), w, namespace)
			return nil
		}, os.Stdout)
}

func PrintRouteTables(routeTables v1.RouteTableList, outputType OutputType) error {
	if outputType == KUBE_YAML {
		return PrintKubeCrdList(routeTables.AsInputResources(), v1.RouteTableCrd)
	}
	return cliutils.PrintList(outputType.String(), "", routeTables,
		func(data interface{}, w io.Writer) error {
			RouteTableTable(data.(v1.RouteTableList), w)
			return nil
		}, os.Stdout)
}

// PrintTable prints virtual services using tables to io.Writer
func VirtualServiceTable(ctx context.Context, list []*v1.VirtualService, w io.Writer, namespace string) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Virtual Service", "Display Name", "Domains", "SSL", "Status", "ListenerPlugins", "Routes"})

	for _, v := range list {
		name := v.GetMetadata().Name
		displayName := v.GetDisplayName()
		domains := domains(v)
		ssl := sslConfig(v)
		status := getStatus(ctx, v, namespace)
		routes := routeList(v.GetVirtualHost().GetRoutes())
		plugins := vhPlugins(v)

		if len(routes) == 0 {
			routes = []string{""}
		}
		for i, line := range routes {
			if i == 0 {
				// Note: table.Append does NOT maintain newlines
				table.Append([]string{name, displayName, domains, ssl, status, plugins, line})
			} else {
				table.Append([]string{"", "", "", "", "", "", line})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

// PrintTable prints virtual services using tables to io.Writer
func RouteTableTable(list []*v1.RouteTable, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Route Table", "Routes", "Status"})

	for _, rt := range list {
		name := rt.GetMetadata().Name
		routes := routeList(rt.GetRoutes())
		status := getRouteTableStatus(rt)

		if len(routes) == 0 {
			routes = []string{""}
		}
		for i, line := range routes {
			if i == 0 {
				table.Append([]string{name, line, status})
			} else {
				table.Append([]string{"", line, ""})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func getRouteTableStatus(vs *v1.RouteTable) string {

	// If the virtual service has not yet been accepted, don't clutter the status with the other errors.
	resourceStatus := vs.GetStatus().GetState()
	if resourceStatus != core.Status_Accepted {
		return resourceStatus.String()
	}

	// Subresource statuses are reported as a map[string]*Status
	// At the moment, virtual services only have one subresource, the associated gateway.
	// In the future, we may add more.
	// Either way, we only care if a subresource is in a non-accepted state.
	// Therefore, only report non-accepted states, include the subresource name.
	subResourceErrorMessages := []string{}
	for k, v := range vs.GetStatus().GetSubresourceStatuses() {
		if v.GetState() != core.Status_Accepted {
			subResourceErrorMessages = append(subResourceErrorMessages, fmt.Sprintf("%v %v: %v", k, v.State.String(), v.Reason))
		}
	}

	switch len(subResourceErrorMessages) {
	case 0:
		// there are no errors with the subresources, pass Accepted status
		return resourceStatus.String()
	case 1:
		// there is one error, try to pass a friendly error message
		return cleanSubResourceError(subResourceErrorMessages[0])
	default:
		// there are multiple errors, don't be fancy, just return list
		return strings.Join(subResourceErrorMessages, "\n")
	}
}

func getStatus(ctx context.Context, res resources.InputResource, namespace string) string {

	// If the virtual service is still pending and may yet be accepted, don't clutter the status with other errors.
	resourceStatus := res.GetStatus().GetState()
	if resourceStatus == core.Status_Pending {
		return resourceStatus.String()
	}

	// Subresource statuses are reported as a map[string]*Status
	// At the moment, virtual services only have one subresource, the associated gateway.
	// In the future, we may add more.
	// Either way, we only care if a subresource is in a non-accepted state.
	subresourceStatuses := res.GetStatus().GetSubresourceStatuses()

	// If the virtual service was accepted, don't include confusing errors on subresources but note if there's another resource potentially blocking config updates.
	if resourceStatus == core.Status_Accepted {
		// if route replacement is turned on, don't say that updates to this resource may be blocked
		settingsClient, err := helpers.SettingsClient(ctx, []string{namespace})
		// if we get any errors, ignore and default to more verbose error message
		if err == nil {
			settings, err := settingsClient.Read(namespace, defaults.SettingsName, clients.ReadOpts{})
			if err == nil && settings.GetGloo().GetInvalidConfigPolicy().GetReplaceInvalidRoutes() {
				return resourceStatus.String()
			}
		}
		for k, v := range subresourceStatuses {
			if v.GetState() != core.Status_Accepted {
				return resourceStatus.String() + "\n" + genericSubResourceMessage(k, v.GetState().String())
			}
		}
		return resourceStatus.String()
	}

	// Only report non-accepted states on subresources, include the subresource name.
	subResourceErrorMessages := []string{}
	for k, v := range subresourceStatuses {
		if v.GetState() != core.Status_Accepted {
			subResourceErrorMessages = append(subResourceErrorMessages, fmt.Sprintf("%v %v: %v", k, v.State.String(), v.Reason))
		}
	}

	var subResourceMessage string
	switch len(subResourceErrorMessages) {
	case 0:
		// there are no errors with the subresources
		return resourceStatus.String()
	case 1:
		// there is one error, try to pass a friendly error message
		subResourceMessage = cleanSubResourceError(subResourceErrorMessages[0])
	default:
		// there are multiple errors, don't be fancy, just return list
		subResourceMessage = strings.Join(subResourceErrorMessages, "\n")
	}

	// Note: Parent function does NOT maintain newlines. Keeping them in case we fix that in the future.
	return resourceStatus.String() + "\n" + subResourceMessage
}

// If we can identify the type of error on a virtual service subresource,
// return a cleaner message. If not, default to the full error message.
func cleanSubResourceError(eMsg string) string {
	// If we add additional error scrubbers, we should use regexs
	// For now, a simple way to test for the known error is to split the full error message by it
	// If the split produced a list with two elements, then the error message is recognized
	parts := strings.Split(eMsg, gloov1.UpstreamListErrorTag)
	if len(parts) == 2 {
		// if here, eMsg ~= "<preamble><well_known_error_string><error_details>"
		errorDetails := parts[1]
		return subResourceErrorFormat(errorDetails)
	}
	return eMsg
}

func routeList(routeList []*v1.Route) []string {
	if len(routeList) == 0 {
		return nil
	}
	var routes []string
	for _, route := range routeList {
		var namePrefix string
		if route.GetName() != "" {
			namePrefix = route.GetName() + ": "
		}
		routes = append(routes, fmt.Sprintf("%s%v -> %v", namePrefix, matchersString(route.GetMatchers()), destinationString(route)))
	}
	return routes
}

func vhPlugins(v *v1.VirtualService) string {
	var pluginStr string
	if v.GetVirtualHost().GetOptions() != nil {
		// TODO: fill this when there are vhost plugins
	}
	return pluginStr
}

func matchersString(matchers []*matchers.Matcher) string {
	var matchersStrings []string
	for _, matcher := range matchers {
		matchersStrings = append(matchersStrings, matcherString(matcher))
	}
	return strings.Join(matchersStrings, ", ")
}

func matcherString(matcher *matchers.Matcher) string {
	switch ps := matcher.PathSpecifier.(type) {
	case *matchers.Matcher_Exact:
		return ps.Exact
	case *matchers.Matcher_Prefix:
		return ps.Prefix
	case *matchers.Matcher_Regex:
		return ps.Regex
	}
	return ""
}

func destinationString(route *v1.Route) string {
	switch action := route.Action.(type) {
	case *v1.Route_RouteAction:
		switch dest := action.RouteAction.Destination.(type) {
		case *gloov1.RouteAction_Multi:
			return fmt.Sprintf("%v destinations", len(dest.Multi.Destinations))
		case *gloov1.RouteAction_Single:
			switch destType := dest.Single.DestinationType.(type) {
			case *gloov1.Destination_Upstream:
				return fmt.Sprintf("%s (upstream)", destType.Upstream.Key())
			case *gloov1.Destination_Kube:
				return fmt.Sprintf("%s (service)", destType.Kube.Ref.Key())
			}
		case *gloov1.RouteAction_UpstreamGroup:
			return fmt.Sprintf("upstream group: %s.%s", dest.UpstreamGroup.Name, dest.UpstreamGroup.Namespace)
		}
	case *v1.Route_DirectResponseAction:
		return strconv.Itoa(int(action.DirectResponseAction.Status))
	case *v1.Route_RedirectAction:
		return action.RedirectAction.HostRedirect
	case *v1.Route_DelegateAction:
		if delegateSingle := action.DelegateAction.GetRef(); delegateSingle != nil {
			return fmt.Sprintf("%s (route table)", delegateSingle.Key())
		}
	}
	return ""
}

func domains(v *v1.VirtualService) string {
	if v.VirtualHost.Domains == nil || len(v.VirtualHost.Domains) == 0 {
		return ""
	}

	return strings.Join(v.VirtualHost.Domains, ", ")
}

func sslConfig(v *v1.VirtualService) string {
	if v.GetSslConfig() == nil {
		return "none"
	}

	switch v.GetSslConfig().SslSecrets.(type) {
	case *gloov1.SslConfig_SecretRef:
		return "secret_ref"
	case *gloov1.SslConfig_SslFiles:
		return "ssl_files"
	case *gloov1.SslConfig_Sds:
		return "sds"
	default:
		return "unknown"
	}
}

func genericErrorFormat(resourceName, statusString, reason string) string {
	return fmt.Sprintf("%v %v: %v",
		strings.TrimSpace(resourceName),
		strings.TrimSpace(statusString),
		strings.TrimSpace(reason))
}
func subResourceErrorFormat(errorDetails string) string {
	return fmt.Sprintf("Error with Route: %v: %v", strings.TrimSpace(gloov1.UpstreamListErrorTag), strings.TrimPrefix(errorDetails, ": "))
}
func genericSubResourceMessage(resourceName, statusString string) string {
	return fmt.Sprintf("%v is in a %v state. Updates to this resource may be blocked by problems with another resource.",
		resourceName, statusString)
}
