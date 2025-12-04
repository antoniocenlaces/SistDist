#!/bin/bash

kubectl delete job raft-boot --ignore-not-found
kubectl delete pod cliente --ignore-not-found
kubectl delete statefulset raft --ignore-not-found
kubectl delete service raft-svc --ignore-not-found

kubectl delete pod --all --force --grace-period=0
kubectl delete pvc --all
kind delete clusters kind
./kind-with-registry.sh
docker ps | grep registry
kubectl apply -f raft-statefulset.yaml
sleep 15
kubectl apply -f boot.yaml
sleep 60
kubectl apply -f cliente-pod.yaml
