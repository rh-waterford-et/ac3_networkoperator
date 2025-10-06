# AC3 Network Operator - Project Context

## Overview
The AC3 Network Operator is a Kubernetes operator built with Kubebuilder that manages multi-cluster networking using Skupper service mesh. It enables secure, application-layer communication between applications running across different Kubernetes clusters without requiring VPNs or special firewall rules.

## Project Structure

### Core Components
- **Domain**: `redhat.com`
- **Group**: `ac3`
- **Version**: `v1alpha1`
- **Kind**: `MultiClusterNetwork`
- **Go Module**: `github.com/rh-waterford-et/ac3_networkoperator`
- **Kubebuilder Version**: v4 with go.kubebuilder.io/v4 layout

### Key Files and Directories

#### API Definition
- `api/v1alpha1/ac3network_types.go` - Custom Resource Definition types
- `config/crd/bases/ac3.redhat.com_multiclusternetworks.yaml` - Generated CRD manifest

#### Controller Logic
- `internal/controller/ac3network_controller.go` - Main reconciliation logic
- `cmd/main.go` - Operator entry point

#### Configuration
- `config/` - Kubebuilder generated configuration
  - `crd/bases/` - CRD definitions and various resource manifests
  - `rbac/` - RBAC configurations for operator permissions
  - `manager/` - Operator deployment configuration
  - `samples/` - Example custom resource instances

#### Examples and Testing
- `multi-cluster-network.yaml` - Current CRD file being modified
- `source.yaml` - ManifestWork example for deploying resources to managed clusters
- `example-ac3network.yaml` - Example AC3Network resource

## Custom Resource Definition

### MultiClusterNetwork Spec
```yaml
apiVersion: ac3.redhat.com/v1alpha1
kind: MultiClusterNetwork
metadata:
  name: example-network
  namespace: ac3no
spec:
  links:
    - sourceCluster: "cluster-1"
      targetCluster: "cluster-4"  
      sourceNamespace: "app-namespace-1"
      targetNamespace: "app-namespace-2"
      applications:
        - "hello-backend"
      services:
        - name: "backend"
          port: 8080
```

### Core Data Structures

#### MultiClusterLink
- `sourceCluster` (string) - Name of source cluster context in kubeconfig
- `targetCluster` (string) - Name of target cluster context in kubeconfig
- `sourceNamespace` (string) - Namespace in source cluster
- `targetNamespace` (string) - Namespace in target cluster
- `applications` ([]string) - List of deployment names to annotate with Skupper
- `services` ([]ServicePortPair) - List of services to expose across clusters

#### ServicePortPair
- `name` (string) - Service name
- `port` (int) - Service port to expose

## Controller Architecture

### Reconciliation Flow
1. **Kubeconfig Management**: Reads combined kubeconfig from ConfigMap `combined-kubeconfig` in `sk1` namespace
2. **Link Processing**: Iterates through each link in the MultiClusterNetwork spec
3. **Skupper Site Setup**: Creates `skupper-site` ConfigMaps with Skupper configuration
4. **Token Management**: Manages connection token secrets for cluster-to-cluster authentication
5. **Service Exposure**: Exposes services using two strategies based on service ownership
6. **Cleanup**: Tracks previous state and cleans up removed links

### Key Reconciliation Functions

#### Core Operations
- `Reconcile()` - Main reconciliation loop (runs every 30 seconds)
- `cleanupRemovedLinks()` - Manages cleanup of deleted links
- `createUpdateSecret()` - Creates connection token secrets
- `updateDeploymentsWithSkupperAnnotation()` - Adds Skupper annotations to deployments

#### Service Exposure Strategies
- **Simple Annotation**: For services without `AppliedManifestWork` owner
  - Directly annotates services with `skupper.io/proxy` and `skupper.io/port`
- **Proxy/ExternalName**: For services with `AppliedManifestWork` owner  
  - Creates `-skupper` suffixed proxy services
  - Creates ExternalName services pointing to proxy services

#### Cleanup Functions
- `cleanupSkupperProxyServices()` - Removes Skupper proxy services
- `cleanupExternalNameServices()` - Removes ExternalName services  
- `cleanupSkupperAnnotations()` - Removes Skupper annotations from services
- `cleanupSkupperSiteConfigMaps()` - Removes skupper-site ConfigMaps
- `cleanupTokenSecrets()` - Removes connection token secrets

## Deployment and Operation

### Prerequisites
- Go 1.20+
- Docker 17.03+
- kubectl v1.11.3+
- Access to Kubernetes v1.11.3+ clusters
- Combined kubeconfig with contexts for all target clusters

### Required ConfigMap Setup
```bash
kubectl config view --raw > combined-kubeconfig
kubectl create configmap combined-kubeconfig --from-file=kubeconfig=combined-kubeconfig -n sk1
```

### Build and Deploy
```bash
# Build and push operator image
make docker-build docker-push IMG=<registry>/ac3no:tag

# Install CRDs
make install

# Deploy operator
make deploy IMG=<registry>/ac3no:tag

# Create MultiClusterNetwork instance
kubectl apply -f multi-cluster-network.yaml
```

## RBAC Permissions

The operator requires extensive RBAC permissions:
- Full access to `multiclusternetworks` resources
- Access to `secrets` and `configmaps`
- Cross-cluster access via kubeconfig contexts

## Integration with Red Hat Advanced Cluster Management

### ManifestWork Integration
The operator is designed to work with Red Hat ACM (Advanced Cluster Management):
- Uses ManifestWork resources to deploy applications to managed clusters
- Detects services with `AppliedManifestWork` ownership for special handling
- Example in `source.yaml` shows ManifestWork deploying hello-world-backend

### Multi-Cluster Architecture
- **Hub Cluster**: Runs the AC3 Network Operator
- **Managed Clusters**: Target clusters where applications are deployed
- **Service Mesh**: Skupper provides the underlying connectivity

## Current State and Modifications

### Modified Files (from git status)
- `config/crd/bases/backend.yaml`
- `config/crd/bases/controller-deployment.yaml` 
- `config/crd/bases/multi-cluster-network.yaml` - Currently focused CRD
- `internal/controller/ac3network_controller.go` - Main controller logic
- `source.yaml` - ManifestWork example

### Untracked Files
- `cursor_bypass.py` - Utility script
- `original_code_backup.go` - Backup of controller code
- `utils.py` - Python utilities

## Technical Details

### Container Images
- Operator Image: `ryjenkin/ac3-operator:latest` (per deploy.sh)
- Backend Example: `quay.io/skupper/hello-world-backend`
- Skupper Router: `quay.io/ryjenkin/ac3no3:291`

### Resource Management
- **CPU/Memory Limits**: Configurable via skupper-site ConfigMap
- **Connection Costs**: Tracks connection token costs via annotations
- **Leader Election**: Configured for high availability

### Logging and Monitoring
- Structured logging with context
- Prometheus metrics endpoint (`:8080`)
- Health and readiness probes (`:8081`)
- Optional HTTP/2 support with security considerations

## Future Enhancement Opportunities

### Potential Feature Areas
1. **Enhanced Service Discovery**: Automatic service detection and exposure
2. **Traffic Policies**: Advanced routing and load balancing rules  
3. **Security Policies**: Fine-grained access control between clusters
4. **Observability**: Enhanced monitoring and tracing capabilities
5. **Multi-Tenancy**: Namespace isolation and resource quotas
6. **GitOps Integration**: Declarative configuration management
7. **Disaster Recovery**: Automatic failover and backup strategies
8. **Cost Optimization**: Resource usage monitoring and optimization
9. **Performance Tuning**: Connection pooling and caching
10. **Integration Extensions**: Support for other service mesh technologies

### Architecture Improvements
- **Status Reporting**: Enhanced status fields for better observability
- **Validation**: Admission webhooks for resource validation
- **Conversion**: Version conversion webhooks for API evolution
- **Events**: Kubernetes events for operational visibility
- **Metrics**: Custom metrics for business logic monitoring

## Dependencies

### Go Modules
- Kubernetes client libraries (v0.28.3)
- Controller-runtime (v0.16.3)
- Kubebuilder tooling
- Testing frameworks (Ginkgo/Gomega)

### Operational Dependencies
- Skupper service mesh
- Red Hat ACM (optional but integrated)
- Container registry for operator images
- Kubernetes clusters with appropriate RBAC

---

*This context document represents the current state as of the analysis. It should be updated as the project evolves and new features are added.*
