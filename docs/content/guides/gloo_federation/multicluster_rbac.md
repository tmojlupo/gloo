---
title: Multicluster RBAC
description: Applying role-based access control to multiple Gloo instances
weight: 40
---

Gloo Federation allows you to administer multiple instances of Gloo across multiple Kubernetes clusters. One Gloo Federation object might modify configuration across many instances of Gloo across many Kubernetes clusters. Multicluster role-based access control is a feature of Gloo Federation that controls access and actions on Gloo Federation APIs that might reconfigure many Gloo instances. The feature ensures that users are only allowed to modify Gloo Federation resources that configure Gloo resources in clusters and namespaces that they have explicitly been granted access to in order to facilitate multitenancy in the Gloo Federation control plane.

## Prerequisites

To successfully follow this Multicluster RBAC guide, you will need the following software available and configured on your system.

* Kubectl - Used to execute commands against the clusters
* Glooctl - Used to register the Kubernetes clusters with Gloo Federation
* [Kind](https://kind.sigs.k8s.io/) - Required if using the `glooctl` federation demo environment
* Docker - Required if using the `glooctl` federation demo environment

In this guide we are going to use the Gloo Federation environment available from the `glooctl demo federation` command. You can follow the directions in the [Getting Started guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/getting_started/" %}}) to set up the demonstration environment. Otherwise, you will need at least one Kubernetes cluster running Gloo Enterprise and Gloo Federation.

For the purposes of this example, we have two clusters `local` and `remote`. The local cluster is also running Gloo Federation in addition to Gloo Enterprise. The kubectl context for the local cluster is `kind-local` and the remote cluster is `kind-remote`.

## Enable Multicluster RBAC

Multicluster RBAC can be enabled during Gloo Federation installation by overriding the `enableMultiClusterRbac` value in the helm chart to `true`. To do so, you would run the following `glooctl` command:

```bash
echo "enableMultiClusterRbac: true" > values.yaml

glooctl install federation --values values.yaml --license-key LICENSE_KEY
```

On an existing installation, the `enableMultiClusterRbac` setting can be updated by running `helm upgrade` and overriding the value. Enabling the Multicluster RBAC feature creates an RBAC webhook and pod enforcing permissions on the Gloo Federation API groups.

In the demonstration environment, we will upgrade the existing Gloo Federation installation by running the following:

```bash
# Add the gloo-fed helm repo
helm repo add gloo-fed https://storage.googleapis.com/gloo-fed-helm

# Update your repos 
helm repo update

# Upgrade your Gloo Federation deployment
# Assumes your install is called gloo-fed in the gloo-fed namspace
helm upgrade -n gloo-fed gloo-fed gloo-fed/gloo-fed --set enableMultiClusterRbac=true
```

Once the installation or upgrade is complete, you can verify by running the following:

```bash
kubectl get deployment -n gloo-fed rbac-validating-webhook-gloo-fed
```

You should see the following output:

```bash
NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
rbac-validating-webhook-gloo-fed   1/1     1            1           33m
```

### Examine the Custom Resources
The installation of Multicluster RBAC also creates two MultiClusterRole Custom Resources and two MultiClusterRoleBinding Custom Resources.

```bash
kubectl get multiclusterrole -n gloo-fed 
```

```console
NAME               AGE
gloo-fed           35m
gloo-fed-console   35m
```

```bash
kubectl get multiclusterrolebinding -n gloo-fed
```

```console
NAME               AGE
gloo-fed           35m
gloo-fed-console   35m
```

The `gloo-fed` MutliClusterRole defines a role with permissions to take any actions across all Gloo instances in all clusters. The relevant portion of the spec is shown below:

```yaml
spec:
  rules:
  - apiGroup: fed.solo.io
    placements:
    - clusters:
      - '*'
      namespaces:
      - '*'
  - apiGroup: fed.gloo.solo.io
    placements:
    - clusters:
      - '*'
      namespaces:
      - '*'
  - apiGroup: fed.gateway.solo.io
    placements:
    - clusters:
      - '*'
      namespaces:
      - '*'
  - apiGroup: fed.enterprise.gloo.solo.io
    placements:
    - clusters:
      - '*'
      namespaces:
      - '*'
```

The `gloo-fed` MultiClusterRoleBinding associates the MultiClusterRole with the `gloo-fed` service account. Without the binding, the gloo-fed pod wouldn't be able to update the status of Gloo Federation API objects.

```yaml
spec:
  roleRef:
    name: gloo-fed
    namespace: gloo-fed
  subjects:
  - kind: User
    name: system:serviceaccount:gloo-fed:gloo-fed
```

The `gloo-fed-console` MultiClusterRole and MultiClusterRoleBinding grant the same set of permissions to the `gloo-fed-console` service account.

## Create Roles and Bindings

In the previous section we installed Multicluster RBAC. The process created a MultiClusterRole and MultiClusterRoleBinding for both the `gloo-fed` and `gloo-fed-console` service account. It did not create any role or binding for the default kind user account, kubernetes-admin, which means that we cannot make any changes to the Gloo Federation custom resources.

Let's try and create a FederatedUpstream on the local cluster:

```yaml
kubectl apply -f - <<EOF
apiVersion: fed.gloo.solo.io/v1
kind: FederatedUpstream
metadata:
  name: my-federated-upstream
  namespace: gloo-fed
spec:
  placement:
    clusters:
      - local
    namespaces:
      - gloo-system
  template:
    spec:
      static:
        hosts:
          - addr: solo.io
            port: 80
    metadata:
      name: fed-upstream
EOF
```

We get the following error:

```console
Error from server (User kubernetes-admin does not have the permissions necessary to perform this action.): error when creating "STDIN": admission webhook "rbac-validating-webhook-gloo-fed.gloo-fed.svc" denied the request: User kubernetes-admin does not have the permissions necessary to perform this action.
```

Great! That is what we should expect and shows that Multicluster RBAC is doing its job. Now let's create a new MultiClusterRoleBinding for the `kubernetes-admin` account binding it to the gloo-fed MultiClusterRole.

```yaml
kubectl apply -f - <<EOF
apiVersion: multicluster.solo.io/v1alpha1
kind: MultiClusterRoleBinding
metadata:
  name: kind-admin
  namespace: gloo-fed
spec:
  roleRef:
    name: gloo-fed
    namespace: gloo-fed
  subjects:
  - kind: User
    name: kubernetes-admin
EOF
```

Now if we try and create the same FederatedUpstream again, we'll get the following response:

```console
federatedupstream.fed.gloo.solo.io/my-federated-upstream created
```

### Create a New MultiClusterRole

We may want to restrict an account to perform specific actions on a specific cluster and namespace. We can do this by creating a new MultiClusterRole and then binding it to a service account or user. First, let's create a new role.

```yaml
kubectl apply -f - <<EOF
apiVersion: multicluster.solo.io/v1alpha1
kind: MultiClusterRole
metadata:
  name: remote-admin
  namespace: gloo-fed
spec:
  rules:
  - apiGroup: fed.solo.io
    action: CREATE
    placements:
    - clusters:
      - remote
      namespaces:
      - gloo-system
  - apiGroup: fed.gloo.solo.io
    action: CREATE
    placements:
    - clusters:
      - remote
      namespaces:
      - gloo-system
  - apiGroup: fed.enterprise.gloo.solo.io
    action: CREATE
    placements:
    - clusters:
      - remote
      namespaces:
      - gloo-system
  - apiGroup: fed.gateway.solo.io
    action: CREATE
    placements:
    - clusters:
      - remote
      namespaces:
      - gloo-system
EOF
```

The MultiClusterRole allows the `CREATE` action for all Gloo Federation API groups on the remote cluster and namspace gloo-system. Any account bound to this role would have no permissions on the local cluster, and would only be able to create new items in the remote cluster. The `action` key of the spec can be set to CREATE, UPDATE, or DELETE.

The next step is to create a service account and bind it to the role. We'll create an account by running the following:

```bash
kubectl create serviceaccount remote-admin -n gloo-fed
```

Now we will create the binding between the MultiClusterRole and the service account:

```yaml
kubectl apply -f - <<EOF
apiVersion: multicluster.solo.io/v1alpha1
kind: MultiClusterRoleBinding
metadata:
  name: remote-admin
  namespace: gloo-fed
spec:
  roleRef:
    name: remote-admin
    namespace: gloo-fed
  subjects:
  - kind: User
    name: system:serviceaccount:gloo-fed:remote-admin
EOF
```

It is possible to set multiple subjects in the binding if necessary. We can check on the binding by running:

```bash
kubectl get multiclusterrolebinding -n gloo-fed remote-admin -oyaml
```

You can customize both the MultiClusterRole and MultiClusterRoleBindings to match your unique requirements. Since the RBAC model is deny by default, any access not explicitly granted will be denied.

## Next Steps

To get deeper into Federated Configuration, you can check out our [guide on the topic]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) next, or learn more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Federation.
