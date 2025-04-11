Create a configmap map with the name `ac3-combined-kubeconfig`

Retrieve Kubeconfig with the command
`kubectl config view --raw > combined-kubeconfig`

Create config map with data
kubectl create configmap ac3-combined-kubeconfig --from-file=kubeconfig=combined-kubeconfig