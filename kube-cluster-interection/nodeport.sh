#!/bin/bash

# this cmd will get all the nodeport in the cluster
kubectl get services --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name}:{.spec.ports[*].nodePort}{"\n"}{end}'
