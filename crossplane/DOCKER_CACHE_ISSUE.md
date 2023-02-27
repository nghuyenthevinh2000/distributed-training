# DOCKER CACHE

There is a limit to docker cache:
1. Images: 
2. Containers
3. Volumes
4. Build Cache

```
docker system df
```

During k8s deployment, I am trying to deploy a pod for aws service through localplane and a provider of crossplane/provider-aws. The crossplane-provider-aws keeps crashing.

Through `kubectl get providers`, it indicates that HEALTHY status is false for crossplane-provider-aws. I further check with `kubectl describe service crossplane-provider-aws` and get the warning of out of space. Further investigation indicates that docker volume cache is the problem source here. Pruning docker volume cache solves this. Provider crossplane-provider-aws can run normally with HEALTHY status true.