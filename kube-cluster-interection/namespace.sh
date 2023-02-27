#!/bin/bash
# in this example, crossplane-system is the namespace. Replace crossplane-system with different namespace

# describing a namespace in kube cluster
kubectl describe namespace crossplane-system

# list all namespace in kube cluster
kubectl get namespaces

# delete a namespace in kube cluster
kubectl delete namespace crossplane-system