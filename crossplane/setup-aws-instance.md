# Setup an AWS instance

## Terminology
1. localstack: a cloud service emulator that runs in a single container on your laptop or in your CI environment

2. kind (Kubernetes in Docker): a tool for running local Kubernetes clusters using Docker container "nodes". It provides a lightweight and easy-to-use way to set up a Kubernetes environment on your local machine for development and testing purposes.

3. helm: package manager for Kubernetes that helps users manage, install, and upgrade applications on a Kubernetes cluster. Helm provides a command-line interface (CLI) that __allows users to interact with the Kubernetes cluster__ using pre-built charts, which are packages that contain all the necessary Kubernetes manifests, templates, and configurations to deploy an application.

4. chart: a package that contains all the necessary Kubernetes manifests, templates, and configurations to deploy an application on a Kubernetes cluster.

5. namespace: In Kubernetes, a namespace is a way to divide and isolate resources within a cluster. A namespace provides a way to create a virtual cluster inside a physical cluster. Resources created in one namespace are hidden from resources created in another namespace.

By using namespaces, you can create logical divisions within a cluster and ensure that resources created by different teams or applications don't conflict with each other. For example, you might create one namespace for development, another for testing, and another for production. Each namespace would contain its own set of resources, such as pods, services, and deployments.

6. nodePort: a port number that is allocated on each node in the cluster and can be used to access the service from outside the cluster. It must be a valid TCP port number that is not already in use by another service or process on the node.

7. service: an abstraction layer that provides a stable IP address and DNS name for a set of pods in a deployment. A Service acts as a single entry point for accessing one or more instances of an application. It provides load balancing and high availability by automatically routing traffic to healthy pods and ensuring that traffic is not sent to pods that are unhealthy or not ready. Services can be exposed within a cluster, or externally, using a variety of networking options.

8. pod: the smallest and simplest unit in the Kubernetes object model that represents a single instance of a running process in a cluster. A Pod encapsulates one or more containers, storage resources, a unique network IP, and options that govern how the container(s) should run. A Pod can contain a single container or multiple containers that share the same network namespace and can communicate with each other using localhost. Pods are typically used to deploy and manage stateless or stateful applications.

9. provider: type of custom resource definition (CRD) in Crossplane that allows you to specify information about an external service or infrastructure provider, such as AWS, GCP, or Azure. It is used to define the connection to the external provider, including authentication credentials and connection information. Providers can be used by other Kubernetes resources, such as Workloads, to access the external service or infrastructure.

## How it works
1. setup a local kube cluster in docker
2. mount crossplane onto that local kube cluster
3. mount localplane onto that local kube cluster