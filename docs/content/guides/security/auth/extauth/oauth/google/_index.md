---
title: Authenticate with Google
weight: 10
description: Setup OpenID Connect (OIDC) authentication with the Google identity provider. 
---

In this guide we will see how to authenticate users with your application by allowing them to log in to their Google 
account. This guide is just an example to get you started and does not cover all aspects of a complete setup, 
like setting up a domain and SSL certificates.

## Setup
{{% notice warning %}}
This feature requires Gloo's external auth server to communicate with an external OIDC provider/authorization server.
Because of this interaction, the OIDC flow may take longer than the default timeout of 200ms.
You can increase this timeout by setting the {{% protobuf name="enterprise.gloo.solo.io.Settings" display="`requestTimeout` value on external auth settings"%}}.
The external auth settings can be configured on the {{% protobuf name="gloo.solo.io.Settings" display="global Gloo `Settings` object"%}}.
{{% /notice %}}

{{< readfile file="/static/content/setup_notes" markdown="true">}}

### Deploy sample application
{{% notice warning %}}
The sample `petclinic` application deploys a MySql server. If you are using `minikube` v1.5 to run this guide, this 
service is likely to crash due a `minikube` [issue](https://github.com/kubernetes/minikube/issues/5751). 
To get around this, you can start `minikube` with the following flag:

```shell
minikube start --docker-opt="default-ulimit=nofile=102400:102400" 
```
{{% /notice %}}

Let's deploy a sample web application that we will use to demonstrate these features:
```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```

### Creating a Virtual Service
Now we can create a Virtual Service that routes all requests (note the `/app` prefix) to the `petclinic` service.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /app
      routeAction:
        single:
          kube:
            ref:
              name: petclinic
              namespace: default
            port: 80
      options:
        prefixRewrite: '/'
```

To verify that the Virtual Service has been accepted by Gloo, let's port-forward the Gateway Proxy service so that it is 
reachable from you machine at `localhost:8080`:
```
kubectl -n gloo-system port-forward svc/gateway-proxy 8080:80
```

If you open your browser and navigate to [http://localhost:8080/app](http://localhost:8080/app) you should see the following page (you might need to wait a minute for the containers to start):

![Pet Clinic app homepage](petclinic-home.png)

## Securing the Virtual Service
As we just saw, we were able to reach our application without having to provide any credentials. This is because by default Gloo allows any request on routes that do not specify authentication configuration. Let's change this behavior. We will update the Virtual Service so that each request to the sample application is authenticated using an **OpenID Connect** flow.

### Register your application with Google
In order to use Google as our identity provider, we need to register our application with the Google API.
To do so:
 
- Log in to the [Google Developer Console](https://console.developers.google.com/)
- If this is the first time using the console, create a [project](https://cloud.google.com/resource-manager/docs/creating-managing-projects)
as prompted;
- Navigate to the [OAuth consent screen](https://console.developers.google.com/apis/credentials/consent) menu item
- Input a name for your application in the *Application name* text field and select **Internal** as the *Application type*
- Click **Save**;
- Navigate to the [Credentials](https://console.developers.google.com/apis/credentials) menu item
- click **Create credentials**, and then **OAuth client ID**
- On the next page, select *Web Application* as the type of the client (as we are only going to use it for demonstration purposes), 
- Enter a name for the OAuth client ID or accept the default value
- Under *Authorized redirect URIs* click on **Add URI**
- Enter the URI: `http://localhost:8080/callback`
- Click **Create**

You will be presented with the **client id** and **client secret** for your application.
Let's store them in two environment variables:

```noop
CLIENT_ID=<your client id>
CLIENT_SECRET=<your client secret>
```

### Create a client ID secret
Gloo expects the client secret to stored in a Kubernetes secret. Let's create the secret with the value of our `CLIENT_SECRET` variable:

```shell
glooctl create secret oauth --namespace gloo-system --name google --client-secret $CLIENT_SECRET
```

### Create an AuthConfig
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

Now let's create the `AuthConfig` resource that we will use to secure our Virtual Service.

{{< highlight shell "hl_lines=9-16" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: google-oidc
  namespace: gloo-system
spec:
  configs:
  - oauth2:
      oidcAuthorizationCode:
        app_url: http://localhost:8080
        callback_path: /callback
        client_id: $CLIENT_ID
        client_secret_ref:
          name: google
          namespace: gloo-system
        issuer_url: https://accounts.google.com
        scopes:
        - email
EOF
{{< /highlight >}}

{{% notice note %}}
The above configuration uses the new `oauth2` syntax. The older `oauth` syntax is still supported, but has been deprecated.
{{% /notice %}}

Notice how we set the `CLIENT_ID` and reference the client secret we just created. The `callback_path` matches the authorized redirect URI we added for the OAuth Client ID. Redirecting to an unauthorized URI would result in an error from the Google authentication flow.

### Update the Virtual Service
Once the AuthConfig has been created, we can use it to secure our Virtual Service:

{{< highlight yaml "hl_lines=11-21" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /callback
      routeAction:
        single:
          kube:
            ref:
              name: petclinic
              namespace: default
            port: 80
      options:
        prefixRewrite: '/login'
    - matchers:
      - prefix: /app
      routeAction:
        single:
          kube:
            ref:
              name: petclinic
              namespace: default
            port: 80
      options:
          prefixRewrite: '/'
    options:
      extauth:
        configRef:
          name: google-oidc
          namespace: gloo-system
{{< /highlight >}}

{{% notice note %}}
This example is sending the `/callback` prefix to `/login`, a path that does not exist. The request will not be interpreted by the petclinic service, but you could easily add code for the `/login` path that would parse the state information from Google and use it to load a profile of the user.
{{% /notice %}}

## Testing our configuration
Since we didn't register an external URL, Google will only allow authentication with applications running on localhost for security reasons. We can make the Gloo Gateway available on localhost using `kubectl port-forward`:

```shell
kubectl port-forward -n gloo-system deploy/gateway-proxy 8080 &
portForwardPid=$! # Store the port-forward pid so we can kill the process later
```

Now if you open your browser and go to http://localhost:8080/app you should be redirected to the Google login screen:

![Google login page](google-login.png)
 
If you provide your Google credentials, Gloo should redirect you to the `/callback` page, with the information from Google added as a query string.

![Pet Clinic app homepage](petclinic-querystring.jpeg)

If this does not work, one thing to check is the `requestTimeout` setting on your `extauth` Settings. See the warning in the [setup section](#setup) for more details.

### Logging

If Gloo is running on kubernetes, the extauth server logs can be viewed with:
```
kubectl logs -n gloo-system deploy/extauth -f
```
If the auth config has been received successfully, you should see the log line:
```
"logger":"extauth","caller":"runner/run.go:179","msg":"got new config"
```

## Cleanup
To clean up the resources we created during this tutorial you can run the following commands:

```bash
kill $portForwardPid
kubectl delete virtualservice -n gloo-system petclinic
kubectl delete authconfig -n gloo-system google-oidc
kubectl delete secret -n gloo-system google
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```