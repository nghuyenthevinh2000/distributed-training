https://www.copado.com/devops-hub/blog/kubernetes-deployment-vs-service-managing-your-pods

# Service

A service is a method for exposing a network application that is running as one or more Pods in your cluster. Basically, it provides accessibility to pods.

1. To quickly create a service for a pod: `kubectl expose pod <pod name>`

you can now access pod api services through: `http://<pod name>:80`

2. Debug service guide: https://kubernetes.io/docs/tasks/debug/debug-application/debug-service/

3. There are four types of Kube services:
* ClusterIP: a ClusterIP is a virtual IP address that is assigned to a Kubernetes service. The ClusterIP is used by other services within the same Kubernetes cluster to access the service.
  * A DNS service in Kube (CoreDNS) will translate a service, pod name to their ClusterIP for easier usage
* NodePort (reserve a specific pod, may lead to pod conflicts)
* Load Balancer: load balancer exposes a k8s service to external network
* ExternalName

## More information
1. [debug service guide](https://kubernetes.io/docs/tasks/debug/debug-application/debug-service/)