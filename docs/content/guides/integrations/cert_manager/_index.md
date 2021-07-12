---
title: "Integrating Gloo Edge and Let's Encrypt with cert-manager"
menuTitle: Cert-manager
description: Secure your ingress traffic using Gloo Edge and cert-manager
weight: 20
---

This document shows how to secure your traffic using Gloo Edge, Let's Encrypt, and cert-manager. This guide assumes you already 
have a Kubernetes cluster up and running. Further, it assumes your cluster has a load-balancer such that when 
Gloo Edge is installed, the proxy service is granted an external IP. This guide will show examples for both the DNS-01 and HTTP-01 challenges.

## Table of Contents
- [Prerequisites](#prerequisites)
- [DNS-01 Challenge](#utilizing-the-acme-dns-01-challenge)
- [HTTP-01 Challenge](#utilizing-the-acme-http-01-challenge)

---

## Prerequisites

### Install Gloo Edge

To install gloo, run:
```shell
glooctl install gateway
```

### Install cert manager

The official installation guide is [here](https://cert-manager.io/docs/installation/). 
You can install with static manifests or helm. For this example we will use the short version - static manifests:

```shell
kubectl create namespace cert-manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.14.3/cert-manager-legacy.yaml
```

---

## Utilizing the ACME DNS-01 Challenge

### Setup your DNS

In this example we used the domain name `test-123456789.solo.io`. We'll create an `A` record that maps to the IP address of the 
gateway proxy that we installed with Gloo Edge.  

You can run these commands to update AWS route53 through the AWS command line tool 
(remember to replace *HOSTED_ZONE* and *RECORD* with your values):

```shell
export GLOO_HOST=$(kubectl get svc -l gloo=gateway-proxy -n gloo-system -o 'jsonpath={.items[0].status.loadBalancer.ingress[0].ip}')
RECORD=test-123456789
HOSTED_ZONE=solo.io.
ZONEID=$(aws route53 list-hosted-zones|jq -r '.HostedZones[]|select(.Name == "'"$HOSTED_ZONE"'").Id')
RS='{ "Changes": [{"Action": "UPSERT", "ResourceRecordSet":{"ResourceRecords":[{"Value": "'$GLOO_HOST'"}],"Type": "A","Name": "'$RECORD.$HOSTED_ZONE'","TTL": 300} } ]}'
aws route53 change-resource-record-sets --hosted-zone-id $ZONEID --change-batch "$RS"
```

### Add a Service

Add a service that will get exposed via Gloo Edge. In this document we will use the petclinic spring application. 

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```

### Configure access to AWS

We'll need to allow cert manager access to configure DNS records in AWS. See cert manager [docs](https://docs.cert-manager.io/en/latest/tasks/acme/configuring-dns01/route53.html) for more details on the access requirements for cert-manager. 

Once you have configured access, we will need to add the access keys as a kubernetes secret, so that cert manager can access them:

```shell
export ACCESS_KEY_ID=...
export SECRET_ACCESS_KEY=...
kubectl create secret generic aws-creds -n cert-manager --from-literal=access_key_id=$ACCESS_KEY_ID --from-literal=secret_access_key=$SECRET_ACCESS_KEY
```

### Create a cluster issuer
Create a cluster issuer for Let's Encrypt with Route 53.

```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
  namespace: gloo-system
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: yuval@solo.io
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
    - dns01:
        route53:
          region: us-east-1
          accessKeyID: $(kubectl -n cert-manager get secret aws-creds -o=jsonpath='{.data.access_key_id}'|base64 --decode)
          secretAccessKeySecretRef:
            name: aws-creds
            key: secret_access_key
EOF
```

Wait until the cluster issuer is in ready state:

```
kubectl get clusterissuer letsencrypt-staging -o jsonpath='{.status.conditions[0].type}{"\n"}'
Ready
```

### Create a certificate for our service

Create the certificate for the Gloo Edge ingress:
```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-123456789.solo.io
  namespace: gloo-system
spec:
  secretName: test-123456789.solo.io
  dnsNames:
  - test-123456789.solo.io
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
EOF
```

Wait a bit and you will see the secret created:
```shell
kubectl -n gloo-system  get secret
NAME                   TYPE                                  DATA      AGE
test-123456789.solo.io kubernetes.io/tls                     2         3h
...
```

Now just create a virtual host with the same secret ref as the name.

---

### Expose the service securly via Gloo Edge

Configure Gloo Edge's default Virtual Service to route to the function and use the certificates.

```shell
cat <<EOF | kubectl create -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic-ssl
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - test-123456789.solo.io
    routes:
    - matchers:
       - prefix: /
      route_action:
        single:
          upstream:
              name: default-petclinic-80
              namespace: gloo-system
  ssl_config:
    secret_ref:
      name: test-123456789.solo.io
      namespace: gloo-system
EOF
```

---

### Test!

Now we can open the petclinic application at `https://test-123456789.solo.io/`. 

---

## Utilizing the ACME HTTP-01 Challenge

We just explored how to utilize cert-manager to solve the DNS-01 ACME challenge. While that works great, sometimes a "lighter-weight" solution is desirable. For these situations, the HTTP-01 ACME challenge is a good fit.

We will now illustrate solving the HTTP-01 ACME challenge with Gloo Edge and cert-manager. The HTTP-01 challenge specifically involves the ACME server (Let's Encrypt) passing a token to your ACME client (cert-manager). The expectation is for that token to be reachable on your domain at a "well known" path, specifically `http://<YOUR_DOMAIN>/.well-known/acme-challenge/<TOKEN>`

For this example, we will be using an externally accessible IP (provided through a `LoadBalancer` `Service` in a cloud environment) in conjunction with a [nip.io](https://nip.io/) domain name. [nip.io](https://nip.io/) is a helpful service which allows us to map an arbitrary IP address to a specific domain name via DNS.

{{% notice note %}}
These steps are specific for Gloo Edge running in gateway mode. When running in ingress mode, since cert-manager will automatically create `Ingress` resources, you will not need to add/modify `VirtualService` resources.
{{% /notice %}}


### Create an Issuer

First, create a `ClusterIssuer` which will utilize the `http01` solver:
```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging-http01
  namespace: gloo-system
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: law@solo.io
    privateKeySecretRef:
      name: letsencrypt-staging-http01
    solvers:
    - http01:
        ingress:
          serviceType: ClusterIP
      selector:
        dnsNames:
        - $(glooctl proxy address | cut -f 1 -d ':').nip.io
EOF
```

Notice the use of the `http01` solver. By default, cert-manager will create a `Service` of type `NodePort` to be routed via an `Ingress`. However, since we are running Gloo Edge in gateway mode, incoming traffic is routed via a `VirtualService` and does not require a `NodePort`, so we are explicitly setting the `serviceType` to `ClusterIP`. 

Additionally, we are specifying the `dnsName` to be a [nip.io](https://nip.io/) subdomain with the IP of our external facing LoadBalancer IP. The inline command uses `glooctl proxy address` to get the external facing IP address of our proxy and we append the 'nip.io' domain, leaving us with a domain that looks something like: `34.71.xx.xx.nip.io`.

### Create the Certificate

Next we will create the actual `Certificate` which will utilize the `ClusterIssuer` we just created:

```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: nip-io
  namespace: default
spec:
  secretName: nip-io-tls
  issuerRef:
    kind: ClusterIssuer
    name: letsencrypt-staging-http01
  commonName: $(glooctl proxy address | cut -f 1 -d ':').nip.io
  dnsNames:
    - $(glooctl proxy address | cut -f 1 -d ':').nip.io
EOF
```

Once this `Certificate` resource is created, behind the scenes cert-manager will create the relevant `CertificateRequest` and `Order` resources. To satisfy this 'order', cert-manager will spin up a pod and service that will present the correct token.

### Routing to the cert-manager pod

Now that the pod which will serve this token is created, we need to configure Gloo Edge to route to it. In this case, we will create a Virtual Service for our custom domain that will route requests for the path `/.well-known/acme-challenge/<TOKEN>` to the cert-manager created pod.

We can see this pod present in our `default` namespace:
```shell
% kubectl get pod
NAME                        READY   STATUS    RESTARTS   AGE
cm-acme-http-solver-s69mw   1/1     Running   0          1m6s
```

And the `Service` that corresponds to it:
```shell
% kubectl get service
NAME                        TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)                               AGE
cm-acme-http-solver-f6mdb   ClusterIP      10.35.254.161   <none>           8089/TCP                              2m5s
```

With Upstream discovery enabled, an Upstream to this `Service` will automatically be generated:
```shell
% glooctl get us default-cm-acme-http-solver-f6mdb-8089
+----------------------------------------+------------+----------+--------------------------------+
|                UPSTREAM                |    TYPE    |  STATUS  |            DETAILS             |
+----------------------------------------+------------+----------+--------------------------------+
| default-cm-acme-http-solver-f6mdb-8089 | Kubernetes | Accepted | svc name:                      |
|                                        |            |          | cm-acme-http-solver-f6mdb      |
|                                        |            |          | svc namespace: default         |
|                                        |            |          | port:          8089            |
|                                        |            |          |                                |
+----------------------------------------+------------+----------+--------------------------------+
```

In order to view the `token` value for this `Order`, we can inspect the `Order` itself:
```shell
kubectl get orders.acme.cert-manager.io nip-io-556035424-1317610542 -o=jsonpath='{.status.authorizations[0].challenges[?(@.type=="http-01")].token}'
q5x9q1C4pPg1RtDEiXK9aMAb9ExpepU4Pp14pGKDPXo
```

Now we have all the information necessary to create a Virtual Service to route to this pod at the expected path:

```shell
cat << EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: letsencrypt
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - $(glooctl proxy address | cut -f 1 -d ':').nip.io
    routes:
    - matchers:
      - exact: /.well-known/acme-challenge/q5x9q1C4pPg1RtDEiXK9aMAb9ExpepU4Pp14pGKDPXo
      routeAction:
        single:
          upstream:
            name: default-cm-acme-http-solver-f6mdb-8089
            namespace: gloo-system
EOF
```

Note that we are specifying the domain to be our [nip.io](https://nip.io/) domain and routing requests for the path that Let's Encrypt expects, `/.well-known/acme-challenge/<TOKEN>` to the correct Upstream.

Now that the server can successfully reach the pod, the challenge should be complete, and our `Certificate` will be available for use:

```shell
% kubectl get certificates.cert-manager.io
NAME     READY   SECRET       AGE
nip-io   True    nip-io-tls   10m
```

### Test
First, let's make sure we have the petstore application installed on our cluster:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

Then, we configure our Virtual Service to use our newly created TLS secret and route to the petstore application:
```shell
cat << EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: letsencrypt
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - $(glooctl proxy address | cut -f 1 -d ':').nip.io
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
  sslConfig:
    secretRef:
      name: nip-io-tls
      namespace: default
EOF
```

Now we can `curl` the service:
```shell
% curl https://$(glooctl proxy address | cut -f 1 -d ':').nip.io/api/pets -k
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Notice we use the `-k` flag so curl will not verify the certificate, which is necessary as the certificate we generated was from Let's Encrypt's "staging" CA, which is not trusted by our system.

Finally, we can inspect the certificate being presented by Envoy for this route:
```shell
% openssl s_client -connect $(glooctl proxy address | cut -f 1 -d ':').nip.io:443
```

You should see information regarding the certificate used for this connection. Specifically, you should see something similar to the following:

```
subject=/CN=34.71.xx.xx.nip.io
issuer=/CN=Fake LE Intermediate X1
```

We have just confirmed that the service is accessible over the HTTPS port and the certificate from Let's Encrypt has been presented!
