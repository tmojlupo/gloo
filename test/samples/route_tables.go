package samples

import (
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func LinkedRouteTablesWithVirtualService(vsName, namespace string) (*v1.VirtualService, v1.RouteTableList) {
	return LenLinkedRouteTablesWithVirtualService(3, vsName, namespace)
}

func LenLinkedRouteTablesWithVirtualService(lengthOfChain int, vsName, namespace string) (*v1.VirtualService, v1.RouteTableList) {
	root := "/root"
	prefix := root + "/0"

	makeRt := func(i int) *v1.RouteTable {
		return &v1.RouteTable{
			Metadata: &core.Metadata{Name: fmt.Sprintf("node-%d", i), Namespace: namespace},
			Routes: []*v1.Route{{
				Name: "testRouteName",
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: prefix,
					},
				}},
			},
			}}
	}

	routeTables := v1.RouteTableList{
		makeRt(0),
	}
	// append a chain of route tables
	for i := 1; i < lengthOfChain; i++ {
		prefix += fmt.Sprintf("/%d", i)

		routeTables = append(routeTables, makeRt(i))

		// set delegate of previous to what we appended
		ref := routeTables[i].Metadata.Ref()
		routeTables[i-1].Routes[0].Action = &v1.Route_DelegateAction{
			DelegateAction: &v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Ref{
					Ref: ref,
				},
			},
		}
	}

	// append the leaf
	leaf := &v1.RouteTable{
		Metadata: &core.Metadata{Name: "leaf", Namespace: namespace},
		Routes: []*v1.Route{
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Exact{
						Exact: prefix + "/exact",
					},
				}},
				Action: &v1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{}},
			},
		},
	}

	leafRef := leaf.Metadata.Ref()

	routeTables[lengthOfChain-1].Routes[0].Action = &v1.Route_DelegateAction{
		DelegateAction: &v1.DelegateAction{
			DelegationType: &v1.DelegateAction_Ref{
				Ref: leafRef,
			},
		},
	}

	routeTables = append(routeTables, leaf)

	ref := routeTables[0].Metadata.Ref()
	vs := defaults.DefaultVirtualService(namespace, vsName)
	vs.VirtualHost.Routes = []*v1.Route{{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: root,
			},
		}},
		Action: &v1.Route_DelegateAction{
			DelegateAction: &v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Ref{
					Ref: ref,
				},
			},
		},
	}}

	return vs, routeTables
}
