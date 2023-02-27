#!/bin/bash

NODE_PORT=$(kubectl get --namespace aws-localstack -o jsonpath="{.spec.ports[0].nodePort}" services localstack)
NODE_IP=$(kubectl get nodes --namespace aws-localstack -o jsonpath="{.items[0].status.addresses[0].address}")
echo http://$NODE_IP:$NODE_PORT