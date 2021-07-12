---
title: "Building a Proxy Controller for Gloo Edge"
weight: 2
---

In this tutorial, we're going to show how to use Gloo Edge's {{< protobuf name="gloo.solo.io.Proxy" display="Proxy API">}}
to build a router which automatically creates routes for every existing kubernetes service. Then we will show how to
enable the same functionality for consul services as well.

## Why Write a Custom Proxy Controller

Building a `Proxy` [controller](https://kubernetes.io/docs/concepts/architecture/controller/) allows you to add custom
Gloo Edge operational logic to your setup. In this example, we will write a proxy controller that creates and manages a
second Gloo Edge `Proxy` (`my-cool-proxy`) alongside the Gloo Edge managed one (`gateway-proxy`). The `my-cool-proxy` envoy
proxy will route to Gloo Edge discovered kubernetes services using the host header alone, relieving the Gloo Edge admin from
creating virtual services or route tables to route to each discovered service.

Other common use cases that can be solved with custom proxy controllers include:
- automatically creating http routes that redirect to equivalent https routes
- automatically routing to services based on service name and removing the service name prefix from the request path

## How it will work

A custom `Proxy` controller takes any inputs (in our case, Gloo Edge custom resources in kubernetes) and writes the desired
output to managed `Proxy` custom resource(s).

In our case, we will write a controller that takes `Upstream`s and `Proxy`s as inputs and outputs a new `Proxy`. Then
we will deploy the new controller to create and manage our new `my-cool-proxy` `Proxy` custom resource. Finally, we
will deploy a second envoy proxy to kubernetes, have it register to Gloo Edge with its role configured to match the
name of our managed `Proxy` custom resource (`my-cool-proxy`), and configure it to receive configuration from Gloo Edge.

## Writing the Code

{{% notice note %}}
Tested with Gloo Edge v1.6.0-beta16 and Gloo Edge Enterprise v1.6.0-beta10.
{{% /notice %}}

You can view the complete code written in this section [here](https://github.com/solo-io/gloo/tree/master/example/proxycontroller).

If you'd prefer, you can skip to [running the example](#run) and tinker with that as desired rather than follow along below.

## Dependencies

{{% notice note %}}
tested with go version 1.14
{{% /notice %}}

First, initialize a new go module. This should be done in a new directory. The command to initialize an empty go module is:
```shell script
go mod init <your module name here>
```

Once the module has been initialized add the following dependencies to your go.mod file. Kubernetes dependency management
with go.mod can be fairly difficult, this should be all you need to get access to their types, as well as ours.

{{% expand "Click to see the full go.mod file that should be used for this project" %}}
```go
module <your module name here>

go 1.14

require (
	github.com/solo-io/gloo v1.6.0-beta16
	github.com/solo-io/k8s-utils v0.0.3 // indirect
	github.com/solo-io/solo-kit v0.15.2 // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	k8s.io/api => k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.6
	k8s.io/apiserver => k8s.io/apiserver v0.18.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.6
	k8s.io/client-go => k8s.io/client-go v0.18.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.6
	k8s.io/code-generator => k8s.io/code-generator v0.18.6
	k8s.io/component-base => k8s.io/component-base v0.18.6
	k8s.io/cri-api => k8s.io/cri-api v0.18.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.6
	k8s.io/kubectl => k8s.io/kubectl v0.18.6
	k8s.io/kubelet => k8s.io/kubelet v0.18.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.6
	k8s.io/metrics => k8s.io/metrics v0.18.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.6
	k8s.io/utils => k8s.io/utils v0.0.0-20190801114015-581e00157fb1
)
```
{{% /expand %}}

The basis of this `go.mod` file is from the [`Gloo Edge go.mod file`](https://github.com/solo-io/gloo/blob/master/go.mod)




Now that the dependencies are complete (for now), we can move on to the interesting part: writing the controller!

### Initial code

First, we'll start with a `main.go`. We'll use the main function to connect to 
Kubernetes and start an event loop. Start by creating a new `main.go` file in a new directory:

```go
package main

// all the import's we'll need for this controller
import (
	"context"
	"log"
	"os"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchers "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// import for GKE
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)


func main() {}


// make our lives easy
func must(err error) {
	if err != nil {
		panic(err)
	}
}

```

### Gloo Edge API Clients

Then we'll want to use Gloo Edge's libraries to initialize a client for Proxies and Upstreams. Add the following function to your code:

```go

func initGlooClients(ctx context.Context) (v1.UpstreamClient, v1.ProxyClient) {
	// root rest config
	restConfig, err := kubeutils.GetConfig(
		os.Getenv("KUBERNETES_MASTER_URL"),
		os.Getenv("KUBECONFIG"))
	must(err)

	// wrapper for kubernetes shared informer factory
	cache := kube.NewKubeCache(ctx)

	// initialize the CRD client for Gloo Edge Upstreams
	upstreamClient, err := v1.NewUpstreamClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = upstreamClient.Register()
	must(err)

	// initialize the CRD client for Gloo Edge Proxies
	proxyClient, err := v1.NewProxyClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         v1.ProxyCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = proxyClient.Register()
	must(err)

	return upstreamClient, proxyClient
}

```

This function will initialize clients for interacting with Gloo Edge's Upstream and Proxy APIs. 

### Proxy Configuration

Next, we'll define the algorithm for generating Proxy CRDs from a given list of upstreams. In this example, our 
proxy will serve traffic to every service in our cluster. 

Paste the following function into your code. Feel free to modify if you want to get experimental, here's where the 
"opinionated" piece of our controller is defined:

```go

// in this function we'll generate an opinionated
// proxy object with a routes for each of our upstreams
func makeDesiredProxy(upstreams v1.UpstreamList) *v1.Proxy {

	// each virtual host represents the table of routes for a given
	// domain or set of domains.
	// in this example, we'll create one virtual host
	// for each upstream.
	var virtualHosts []*v1.VirtualHost

	for _, upstream := range upstreams {
		upstreamRef := upstream.Metadata.Ref()
		// create a virtual host for each upstream
		vHostForUpstream := &v1.VirtualHost{
			// logical name of the virtual host, should be unique across vhosts
			Name: upstream.Metadata.Name,

			// the domain will be our "matcher".
			// requests with the Host header equal to the upstream name
			// will be routed to this upstream
			Domains: []string{upstream.Metadata.Name},

			// we'll create just one route designed to match any request
			// and send it to the upstream for this domain
			Routes: []*v1.Route{{
				// use a basic catch-all matcher
				Matchers: []*matchers.Matcher{
					&matchers.Matcher{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
					},
				},

				// tell Gloo Edge where to send the requests
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							// single destination
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									// a "reference" to the upstream, which is a Namespace/Name tuple
									Upstream: &upstreamRef,
								},
							},
						},
					},
				},
			}},
		}

		virtualHosts = append(virtualHosts, vHostForUpstream)
	}

	desiredProxy := &v1.Proxy{
		// metadata will be translated to Kubernetes ObjectMeta
		Metadata: core.Metadata{Namespace: "gloo-system", Name: "my-cool-proxy"},

		// we have the option of creating multiple listeners,
		// but for the purpose of this example we'll just use one
		Listeners: []*v1.Listener{{
			// logical name for the listener
			Name: "my-amazing-listener",

			// instruct envoy to bind to all interfaces on port 8080
			BindAddress: "::", BindPort: 8080,

			// at this point you determine what type of listener
			// to use. here we'll be using the HTTP Listener
			// other listener types are currently unsupported,
			// but future
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					// insert our list of virtual hosts here
					VirtualHosts: virtualHosts,
				},
			}},
		},
	}

	return desiredProxy
}

``` 

### Event Loop

Now we'll define a `resync` function to be called whenever we receive a new list of upstreams:

```go

// we received a new list of upstreams! regenerate the desired proxy
// and write it as a CRD to Kubernetes
func resync(ctx context.Context, upstreams v1.UpstreamList, client v1.ProxyClient) {
	desiredProxy := makeDesiredProxy(upstreams)

	// see if the proxy exists. if yes, update; if no, create
	existingProxy, err := client.Read(
		desiredProxy.Metadata.Namespace,
		desiredProxy.Metadata.Name,
		clients.ReadOpts{Ctx: ctx})


	// proxy exists! this is an update, not a create
	if err == nil {

		// sleep for 1s as Gloo Edge may be re-validating our proxy, which can cause resource version to change
		time.Sleep(time.Second)

		// ensure resource version is the latest
		existingProxy, err = client.Read(
			desiredProxy.Metadata.Namespace,
			desiredProxy.Metadata.Name,
			clients.ReadOpts{Ctx: ctx})
		must(err)

		// update the resource version on our desired proxy
		desiredProxy.Metadata.ResourceVersion = existingProxy.Metadata.ResourceVersion
	}

	// write!
	written, err := client.Write(desiredProxy,
		clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})

	must(err)

	log.Printf("wrote proxy object: %+v\n", written)
}


```

### Main Function

Now that we have our clients and a function defining the proxies we'll want to create, all we need to do is tie it 
all together.

Let's set up a loop to watch Upstreams in our main function. Add the following to your `main()` func:

```go

func main() {
	// root context for the whole thing
	ctx := context.Background()

	// initialize Gloo Edge API clients
	upstreamClient, proxyClient := initGlooClients(ctx)

	// start a watch on upstreams. we'll use this as our trigger
	// whenever upstreams are modified, we'll trigger our sync function
	upstreamWatch, watchErrors, initError := upstreamClient.Watch("gloo-system",
		clients.WatchOpts{Ctx: ctx})
	must(initError)

	// our "event loop". an event occurs whenever the list of upstreams has been updated
	for {
		select {
		// if we error during watch, just exit
		case err := <-watchErrors:
			must(err)
		// process a new upstream list
		case newUpstreamList := <-upstreamWatch:
			// we received a new list of upstreams from our watch, 
			resync(ctx, newUpstreamList, proxyClient)
		}
	}
}

```   


### Finished Code

Great! Here's what our completed main file should look like:

{{% expand "Click to see the full main file" %}}
```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// import for GKE
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	// root context for the whole thing
	ctx := context.Background()

	// initialize Gloo Edge API clients, built on top of CRDs
	upstreamClient, proxyClient := initGlooClients(ctx)

	// start a watch on upstreams. we'll use this as our trigger
	// whenever upstreams are modified, we'll trigger our sync function
	upstreamWatch, watchErrors, initError := upstreamClient.Watch("gloo-system",
		clients.WatchOpts{Ctx: ctx})
	must(initError)

	// our "event loop". an event occurs whenever the list of upstreams has been updated
	for {
		select {
		// if we error during watch, just exit
		case err := <-watchErrors:
			must(err)
		// process a new upstream list
		case newUpstreamList := <-upstreamWatch:
			resync(ctx, newUpstreamList, proxyClient)
		}
	}
}

func initGlooClients(ctx context.Context) (v1.UpstreamClient, v1.ProxyClient) {
	// root rest config
	restConfig, err := kubeutils.GetConfig(
		os.Getenv("KUBERNETES_MASTER_URL"),
		os.Getenv("KUBECONFIG"))
	must(err)

	// wrapper for kubernetes shared informer factory
	cache := kube.NewKubeCache(ctx)

	// initialize the CRD client for Gloo Edge Upstreams
	upstreamClient, err := v1.NewUpstreamClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = upstreamClient.Register()
	must(err)

	// initialize the CRD client for Gloo Edge Proxies
	proxyClient, err := v1.NewProxyClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         v1.ProxyCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = proxyClient.Register()
	must(err)

	return upstreamClient, proxyClient
}

// we received a new list of upstreams! regenerate the desired proxy
// and write it as a CRD to Kubernetes
func resync(ctx context.Context, upstreams v1.UpstreamList, client v1.ProxyClient) {
	desiredProxy := makeDesiredProxy(upstreams)

	// see if the proxy exists. if yes, update; if no, create
	existingProxy, err := client.Read(
		desiredProxy.Metadata.Namespace,
		desiredProxy.Metadata.Name,
		clients.ReadOpts{Ctx: ctx})


	// proxy exists! this is an update, not a create
	if err == nil {

		// sleep for 1s as Gloo Edge may be re-validating our proxy, which can cause resource version to change
		time.Sleep(time.Second)

		// ensure resource version is the latest
		existingProxy, err = client.Read(
			desiredProxy.Metadata.Namespace,
			desiredProxy.Metadata.Name,
			clients.ReadOpts{Ctx: ctx})
		must(err)

		// update the resource version on our desired proxy
		desiredProxy.Metadata.ResourceVersion = existingProxy.Metadata.ResourceVersion
	}

	// write!
	written, err := client.Write(desiredProxy,
		clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})

	must(err)

	log.Printf("wrote proxy object: %+v\n", written)
}

// in this function we'll generate an opinionated
// proxy object with a routes for each of our upstreams
func makeDesiredProxy(upstreams v1.UpstreamList) *v1.Proxy {

	// each virtual host represents the table of routes for a given
	// domain or set of domains.
	// in this example, we'll create one virtual host
	// for each upstream.
	var virtualHosts []*v1.VirtualHost

	for _, upstream := range upstreams {

		// create a virtual host for each upstream
		vHostForUpstream := &v1.VirtualHost{
			// logical name of the virtual host, should be unique across vhosts
			Name: upstream.Metadata.Name,

			// the domain will be our "matcher".
			// requests with the Host header equal to the upstream name
			// will be routed to this upstream
			Domains: []string{upstream.Metadata.Name},

			// we'll create just one route designed to match any request
			// and send it to the upstream for this domain
			Routes: []*v1.Route{{
				// use a basic catch-all matcher
				Matchers: &matchers.Matcher{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					},
				},

				// tell Gloo Edge where to send the requests
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							// single destination
							Single: &v1.Destination{
								// a "reference" to the upstream, which is a Namespace/Name tuple
								Upstream: upstream.Metadata.Ref(),
							},
						},
					},
				},
			}},
		}

		virtualHosts = append(virtualHosts, vHostForUpstream)
	}

	desiredProxy := &v1.Proxy{
		// metadata will be translated to Kubernetes ObjectMeta
		Metadata: core.Metadata{Namespace: "gloo-system", Name: "my-cool-proxy"},

		// we have the option of creating multiple listeners,
		// but for the purpose of this example we'll just use one
		Listeners: []*v1.Listener{{
			// logical name for the listener
			Name: "my-amazing-listener",

			// instruct envoy to bind to all interfaces on port 8080
			BindAddress: "::", BindPort: 8080,

			// at this point you determine what type of listener
			// to use. here we'll be using the HTTP Listener
			// other listener types are currently unsupported,
			// but future
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					// insert our list of virtual hosts here
					VirtualHosts: virtualHosts,
				},
			}},
		},
	}

	return desiredProxy
}

// make our lives easy
func must(err error) {
	if err != nil {
		panic(err)
	}
}

```
{{% /expand %}}

### Run

While it's possible to package [this application in a Docker container](https://github.com/solo-io/gloo/tree/master/example/proxycontroller/Dockerfile)
and [deploy it as a pod](https://github.com/solo-io/gloo/tree/master/example/proxycontroller/install/proxycontroller.yaml)
inside of Kubernetes, let's just try running it locally. [Make sure you have Gloo Edge installed]({{% versioned_link_path fromRoot="/installation" %}})
in your cluster so that Discovery will create some Upstreams for us.



{{< tabs >}}
{{< tab name="run locally" codelang="bash">}}
go run example/proxycontroller/proxycontroller.go
{{< /tab >}}
{{< tab name="run in k8s" codelang="bash">}}
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/proxycontroller/install/proxycontroller.yaml
{{< /tab >}}
{{< /tabs >}}

The logs should show:
```
2019/02/11 11:27:30 wrote proxy object: listeners:<name:"my-amazing-listener" bind_address:"::" bind_port:8080 http_listener:<virtual_hosts:<name:"default-kubernetes-443" domains:"default-kubernetes-443" routes:<matchers:<prefix:"/" > route_action:<single:<upstream:<name:"default-kubernetes-443" namespace:"gloo-system" > > > > > virtual_hosts:<name:"gloo-system-gateway-proxy-8080" domains:"gloo-system-gateway-proxy-8080" routes:<matchers:<prefix:"/" > route_action:<single:<upstream:<name:"gloo-system-gateway-proxy-8080" namespace:"gloo-system" > > > > > virtual_hosts:<name:"gloo-system-gloo-9977" domains:"gloo-system-gloo-9977" routes:<matchers:<prefix:"/" > route_action:<single:<upstream:<name:"gloo-system-gloo-9977" namespace:"gloo-system" > > > > > virtual_hosts:<name:"kube-system-kube-dns-53" domains:"kube-system-kube-dns-53" routes:<matchers:<prefix:"/" > route_action:<single:<upstream:<name:"kube-system-kube-dns-53" namespace:"gloo-system" > > > > > virtual_hosts:<name:"kube-system-tiller-deploy-44134" domains:"kube-system-tiller-deploy-44134" routes:<matchers:<prefix:"/" > route_action:<single:<upstream:<name:"kube-system-tiller-deploy-44134" namespace:"gloo-system" > > > > > > > status:<> metadata:<name:"my-cool-proxy" namespace:"gloo-system" resource_version:"455073" > 
```

Neat! Our proxy got created. We can view it with `kubectl`:

```bash
kubectl get proxy -n gloo-system -o yaml
```

{{% expand "Click to see the expected proxy" %}}
```yaml
apiVersion: v1
items:
- apiVersion: gloo.solo.io/v1
  kind: Proxy
  metadata:
    creationTimestamp: "2019-12-20T16:29:37Z"
    generation: 9
    name: my-cool-proxy
    namespace: gloo-system
    resourceVersion: "799"
    selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/proxies/my-cool-proxy
    uid: 58e238af-5b1f-4138-bb63-b8edc2405a6c
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener:
        virtualHosts:
        - domains:
          - default-kubernetes-443
          name: default-kubernetes-443
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: default-kubernetes-443
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-443
          name: gloo-system-gateway-443
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-443
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-proxy-443
          name: gloo-system-gateway-proxy-443
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-proxy-443
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-proxy-80
          name: gloo-system-gateway-proxy-80
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-proxy-80
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-proxy-gateway-proxy-443
          name: gloo-system-gateway-proxy-gateway-proxy-443
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-proxy-gateway-proxy-443
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-proxy-gateway-proxy-80
          name: gloo-system-gateway-proxy-gateway-proxy-80
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-proxy-gateway-proxy-80
                  namespace: gloo-system
        - domains:
          - gloo-system-gloo-9966
          name: gloo-system-gloo-9966
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gloo-9966
                  namespace: gloo-system
        - domains:
          - gloo-system-gloo-9977
          name: gloo-system-gloo-9977
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gloo-9977
                  namespace: gloo-system
        - domains:
          - gloo-system-gloo-9979
          name: gloo-system-gloo-9979
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gloo-9979
                  namespace: gloo-system
        - domains:
          - gloo-system-gloo-9988
          name: gloo-system-gloo-9988
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gloo-9988
                  namespace: gloo-system
        - domains:
          - kube-system-kube-dns-53
          name: kube-system-kube-dns-53
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: kube-system-kube-dns-53
                  namespace: gloo-system
        - domains:
          - kube-system-kube-dns-9153
          name: kube-system-kube-dns-9153
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: kube-system-kube-dns-9153
                  namespace: gloo-system
      name: my-amazing-listener
  status:
    reported_by: gloo
    state: 1
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```
{{% /expand %}}

Cool. Let's leave our controller running and watch it dynamically respond when we add a service to our cluster:

```bash
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

See the service and pod:

```bash
kubectl get pod -n default && kubectl get svc -n default
```

```
NAME                      READY     STATUS    RESTARTS   AGE
petstore-6fd84bc9-zdskz   1/1       Running   0          5s
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
kubernetes   ClusterIP   10.96.0.1       <none>        443/TCP    6d
petstore     ClusterIP   10.109.34.250   <none>        8080/TCP   5s
```

The upstream that was created:

```bash
kubectl get upstream -n gloo-system
```

```
NAME                              AGE
default-kubernetes-443            2m
default-petstore-8080             46s # <- this one's new
gloo-system-gateway-proxy-8080    2m
gloo-system-gloo-9977             2m
kube-system-kube-dns-53           2m
kube-system-tiller-deploy-44134   2m
```

And check that our proxy object was updated:

```bash
kubectl get proxy -n gloo-system -o yaml
```

```yaml
apiVersion: v1
items:
- apiVersion: gloo.solo.io/v1
  kind: Proxy
  metadata:
    creationTimestamp: "2019-12-20T16:29:37Z"
    generation: 12
    name: my-cool-proxy
    namespace: gloo-system
    resourceVersion: "2081"
    selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/proxies/my-cool-proxy
    uid: 58e238af-5b1f-4138-bb63-b8edc2405a6c
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener:
        virtualHosts:
        - domains:
          - default-petstore-8080
          name: default-petstore-8080
          routes:
          - matchers:
            - prefix: /
            routeAction:
              single:
                upstream:
                  name: default-petstore-8080
                  namespace: gloo-system
            ... # <- other virtual hosts omitted for brevity
      name: my-amazing-listener
  status: {}
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""

```

The proxy should have been created with the `default-petstore-8080` virtualHost.

Now that we have a proxy called `my-cool-proxy`, Gloo Edge will be serving xDS configuration that matches this proxy CRD.
However, we don't actually have an Envoy instance deployed that will receive this config. In the next section, 
we'll walk through the steps to deploy an Envoy pod wired to receive config from Gloo Edge, identifying itself as 
`my-cool-proxy`.  


## Deploying Envoy to Kubernetes

Gloo Edge comes pre-installed with at least one proxy depending on your setup: the `gateway-proxy`. This proxy is configured 
by the `gateway` proxy controller. It's not very different from the controller we just wrote!

We'll need to deploy another proxy that will register to Gloo Edge with it's `role` configured to match the name of our proxy 
CRD, `my-cool-proxy`. Let's do it!

If you'd prefer, you can deploy the `my-cool-proxy` envoy configmap, service, and deployment in one step:

```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/proxycontroller/install/my-cool-proxy.yaml
```
and then skip to [testing the proxy](#testing-the-proxy).

### Creating the ConfigMap

Envoy needs a ConfigMap which points it at Gloo Edge as its configuration server. Run the following command to create 
the configmap you'll need:


```bash
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-cool-envoy-config
  namespace: default
data:
  envoy.yaml: |
    node:
      cluster: "1"
      id: "1"
      metadata:

        # this line is what connects this envoy instance to our Proxy crd
        role: "gloo-system~my-cool-proxy"

    static_resources:
      clusters:
      - name: xds_cluster
        connect_timeout: 5.000s
        load_assignment:
          cluster_name: xds_cluster
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:

                    # here's where we provide the hostname of the gloo service
                    address: gloo.gloo-system.svc.cluster.local

                    port_value: 9977
        http2_protocol_options: {}
        type: STRICT_DNS
    dynamic_resources:
      ads_config:
        api_type: GRPC
        grpc_services:
        - envoy_grpc: {cluster_name: xds_cluster}
      cds_config:
        ads: {}
      lds_config:
        ads: {}
    admin:
      access_log_path: /dev/null
      address:
        socket_address:
          address: 127.0.0.1
          port_value: 19000
EOF
```

Note that this will create the configmap in the `default` namespace, but you can run it anywhere. Just make sure 
the proxy deployment and service all go to the same namespace.


### Creating the Service and Deployment

We need to create a `LoadBalancer` service for our proxy so we can connect to it from the outside. Note that 
if you're using a Kubernetes Cluster without an external load balancer (e.g. minikube), we'll be using the service's
`NodePort` to connect.

Run the following command to create the service:

```bash
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  labels:
    gloo: my-cool-proxy
  name: my-cool-proxy
  namespace: default
spec:
  ports:
  - port: 8080 # <- this port should match the port for the HttpListener in our Proxy CRD
    protocol: TCP
    name: http
  selector:
    gloo: my-cool-proxy
  type: LoadBalancer
EOF

```

Finally we'll want to create the deployment itself which will launch a pod with Envoy running inside.

```bash
cat << EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    gloo: my-cool-proxy
  name: my-cool-proxy
  namespace: default
spec:
  replicas: 
  selector:
    matchLabels:
      gloo: my-cool-proxy
  template:
    metadata:
      labels:
        gloo: my-cool-proxy
    spec:
      containers:
      - args: ["--disable-hot-restart"]
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        # image: quay.io/solo-io/gloo-envoy-wrapper:1.2.12 # <- use this instead if using open-source Gloo Edge
        image: quay.io/solo-io/gloo-ee-envoy-wrapper:1.2.0 # <- you must use closed-source envoy if using Gloo Edge Enterprise
        imagePullPolicy: Always
        name: my-cool-proxy
        ports:
        - containerPort: 8080 # <- this port should match the port for the HttpListener in our Proxy CRD
          name: http
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      volumes:
      - configMap:
          name: my-cool-envoy-config
        name: envoy-config
EOF
```

If all went well, we should see our pod starting successfully in `default` (or whichever namespace you picked):

```bash
kubectl get pod -n default
```

```
NAME                             READY     STATUS    RESTARTS   AGE
my-cool-proxy-7bcb58c87d-h4292   1/1       Running   0          3s
petstore-6fd84bc9-zdskz          1/1       Running   0          48m
```

## Testing the Proxy

If you have `glooctl` installed, we can grab the HTTP endpoint of the proxy with the following command: 

```bash
glooctl proxy url -n default --name my-cool-proxy
```

returns

```noop
http://192.168.99.150:30751
```

Using `curl`, we can connect to any service in our cluster by using the correct `Host` header:

```bash
curl $(glooctl proxy url -n default --name my-cool-proxy)/api/pets -H "Host: default-petstore-8080"
```

returns

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Try any `Host` header for any upstream name: 

```bash
kubectl get upstream -n gloo-system
```

```noop
NAME                              AGE
default-kubernetes-443            55m
default-my-cool-proxy-8080        5m
default-petstore-8080             53m
gloo-system-gateway-proxy-8080    55m
gloo-system-gloo-9977             54m
kube-system-kube-dns-53           54m
kube-system-tiller-deploy-44134   54m
```

Sweet! You're an official Gloo Edge developer! You've just seen how easy it is to extend Gloo Edge to service one of many 
potential use cases. Take a look at our 
{{< protobuf name="gloo.solo.io.Proxy" display="API Reference Documentation">}} to learn about the 
wide range of configuration options Proxies expose such as request transformation, SSL termination, serverless computing, 
and much more.

## Appendix - Auto-Generated Routes for Discovered Consul Services

The rest of this guide assumes you've been running on minikube, but this setup can be extrapolated to production/more
complex setups.

Run consul on your local machine:
```shell script
consul agent -dev --client=0.0.0.0
```

Get the Host IP address where consul will be reachable from within minikube pods:
```shell script
minikube ssh "route -n | grep ^0.0.0.0 | awk '{ print \$2 }'"
```

Enable consul service discovery in Gloo Edge, replacing address with the value you got before:
```shell script
kubectl patch settings -n gloo-system default --patch '{"spec": {"consul": {"address": "10.0.2.2:8500", "serviceDiscovery": {}}}}' --type=merge
```

Get the cluster IP for petstore service and register a consul service to point there:
```shell script
PETSTORE_IP=$(kubectl get svc -n default petstore -o=jsonpath='{.spec.clusterIP}')
cat > petstore-service.json <<EOF
{
  "ID": "petstore1",
  "Name": "petstore",
  "Address": "${PETSTORE_IP}",
  "Port": 8080
}
EOF
```

Register the consul service:
```shell script
curl -v \
    -XPUT \
    --data @petstore-service.json \
    "http://127.0.0.1:8500/v1/agent/service/register"
```

Confirm the upstream was discovered:
```shell script
kubectl get us -n gloo-system petstore
```
returns
```noop
NAME       AGE
petstore   8s
```

Hit the discovered consul service:
```shell script
curl $(glooctl proxy url -n default --name my-cool-proxy)/api/pets -H "Host: petstore"
```

returns

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Nice. You've configured Gloo Edge to proactively create routes to discovered consul services!