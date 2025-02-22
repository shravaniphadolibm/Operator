# Golang Operator

## Overview

The Memcached Operator is a Kubernetes operator built using the Operator SDK to manage Memcached instances. It ensures that the number of running Memcached pods matches the desired state defined in a Custom Resource (CR).

## Prerequisites

Before starting, ensure you have the following installed:
```
- Operator SDK v1.5.0+
- Kubectl v1.17.0+
- Podman v3.2.2+
- Golang v1.16.0+
- Access to a container image repository (e.g., Quay.io)
- Admin access to a Kubernetes cluster
```

## Installation

### 1. Set Up the Project

Create a new directory for your project and initialize the Operator SDK:
```
mkdir memcached-operator
cd memcached-operator
operator-sdk init --domain example.com --repo github.com/example/memcached-operator
operator-sdk create api --group cache --version v1alpha1 --kind Memcached --resource --controller
```

### 2. Define the API
```
Edit `api/v1alpha1/memcached_types.go` to define the desired state of Memcached:

// MemcachedSpec defines the desired state of Memcached
type MemcachedSpec struct {
//+kubebuilder:validation:Minimum=0
Size int32 json:"size"
}
// MemcachedStatus defines the observed state of Memcached
type MemcachedStatus struct {
Nodes []string json:"nodes"
}
```

### 3. Generate the manifests:
```
make generate
make manifests
```
### 4. Controller Logic

The reconciliation loop ensures that the Memcached instances match the desired state.
 
1.  Fetch the Memcached resource using the Kubernetes client.
2.  Check the number of Memcached pods running in the cluster.
3.  Compare it with the desired size in `MemcachedSpec`.
4.  If the number of pods is less than desired, create additional pods.
5.  If the number of pods is more than desired, delete extra pods.
6.  Update `MemcachedStatus` with the current pod list.
7.  Requeue if needed based on resource changes or errors.

## Deploying the Operator

### 1. Build and Push the Image

Build the container image and push it to your container registry:
```
podman build -t quay.io/your-repo/memcached-operator:v1 .
podman push quay.io/your-repo/memcached-operator:v1
```

### 2. Deploy to Kubernetes

Deploy the operator to your Kubernetes cluster:
```
make deploy
```

### 3. Create a Memcached Instance

Apply the sample Memcached custom resource to create an instance:
```
kubectl apply -f config/samples/cache_v1alpha1_memcached.yaml
```

### 4. Logging and Debugging

The operator logs reconciliation progress. To view the logs:
```
kubectl logs -f deployment/memcached-operator-controller-manager -n memcached-operator-system
```

### 5. Testing the Operator

Once the operator is deployed on your Kubernetes cluster, change the `Spec.size` in the CR and then check the count of replicaset and the count of pods. It should match the size defined in CR.

Command to edit CR
```
kubectl edit <crd-name> <cr-name>
```

Command to check no of pods
```
kubectl get pods
```

Command to check no of replicaset

```
kubectl get replicaset
```

### 6. Cleanup

To remove the operator and related resources:

```
make undeploy
```
