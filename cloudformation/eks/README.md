# Stellar Disbursement Platform (SDP) AWS Kubernetes (EKS) Deployment Guide

## Prerequisites
- AWS CLI installed and configured
- Helm installed
- kubectl configured to connect to your cluster

## Environment Setup
Before starting, set these environment variables:
```bash
# Required variables
export AWS_REGION=your-region  # e.g., us-west-2, eu-west-1, etc.
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export ENVIRONMENT=dev  # or prod, staging, etc.

# Optional variables (for customizing deployment)
export STACK_NAME_PREFIX=sdp  # Prefix for all CloudFormation stacks
export DOMAIN_NAME=example.org  # Your registered domain
```

## Cloudformation Stacks
This guide walks through deploying the Stellar Disbursement Platform (SDP) infrastructure on AWS. The deployment consists of four CloudFormation stacks that create the necessary infrastructure in a specific order:

- Network Stack (sdp-network-eks.yaml)
  - Creates or uses existing VPC and subnets
  - Sets up networking for both public and private resources
  - Exports used (imported) by database and EKS stack to deploy resources

- Database Stack (sdp-database-eks.yaml)
  - Deploys RDS PostgreSQL database in private subnet
  - Creates necessary database secrets in AWS Secrets Manager

- Keys Stack (sdp-keys-eks.yaml) [Optional]
  - Manages Stellar and encryption keys by either:
    - Using provided keys via parameters, or
    - Auto-generating keys using Lambda function for dev/test environments
  - Stores all keys and secrets in AWS Secrets Manager under /sdp/${ENVIRONMENT}/ path
  - Keys include SEP-10 signing keys, distribution account keys, JWT secrets, etc.

- EKS Stack (sdp-eks.yaml)
  - Creates EKS cluster and node group
  - Sets up IAM roles and security groups
  - Configures IRSA (IAM Roles for Service Accounts)
  - Sets up permissions for pods to access secrets stored in AWS Secrets Manager 

After the CloudFormation stacks are deployed, additional Kubernetes resources are installed via Helm charts to complete the setup. The SDP expects secrets to be available as Kubernetes secrets, but how those secrets are synchronized (whether through ExternalSecrets, direct creation, or other means) is left to the deployer's preference.

Note: Both the Keys stack and ExternalSecrets are optional implementation choices. You can manage and sync secrets to Kubernetes secrets through whatever mechanism best fits your security requirements and operational preferences.

## Change Directory to the EKS Cloudformation Directory
```bash
cd cloudformation/eks
```

## Verify AWS CLI Configuration
```bash
aws configure list
aws sts get-caller-identity
```

## 1. Network Stack Deployment
Deploy the networking infrastructure. Review custom parameters if needed.

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-network \
  --template-body file://sdp-network-eks.yaml \
  --region ${AWS_REGION}
```

## 2. Database Stack Deployment
Deploy the RDS database. Review custom parameters if needed.

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-database \
  --template-body file://sdp-database-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=NetworkStackName,ParameterValue=${STACK_NAME_PREFIX}-network
```

## 3. Keys Stack Deployment
Deploy the secrets and keys management stack:

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION}
```

**Note**: Leave the following parameters empty to auto-generate new keys:
- DistributionSeed
- DistributionPublicKey
- SEP10SigningPrivateKey
- SEP10SigningPublicKey
- ChannelAccountEncryptionPassphrase
- DistributionAccountEncryptionPassphrase

## 4. EKS Cluster Deployment
Deploy the EKS cluster:

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-eks \
  --template-body file://sdp-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=NetworkStackName,ParameterValue=${STACK_NAME_PREFIX}-network \
    ParameterKey=DatabaseStackName,ParameterValue=${STACK_NAME_PREFIX}-database
```

### EKS Configuration and Deployment
The remaining steps will guide you through Kubernetes and Helm deployment steps. This includes:
1. External Secrets Operator installation
2. AWS Secrets Manager access configuration
3. External Secrets creation
4. Nginx Ingress Controller installation
5. Cert-Manager installation
6. External-DNS setup
7. SDP Helm chart deployment

## 5. Configure kubectl
After the EKS cluster is created, configure kubectl:

```bash
aws eks update-kubeconfig \
  --name $(aws cloudformation describe-stacks \
    --stack-name ${STACK_NAME_PREFIX}-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text) \
  --region ${AWS_REGION}
```

Verify you are pointing kubectl to the correct AWS EKS Cluster:
```bash
kubectl config get-contexts
```

## 6. Create Namespace
```bash
kubectl create namespace sdp
```

## 7. External Secrets Operator Installation
```bash
# Create external-secrets namespace
kubectl create namespace external-secrets

# Add and update Helm repository
helm repo add external-secrets https://charts.external-secrets.io
helm repo update

# Install External Secrets Operator
helm install external-secrets external-secrets/external-secrets \
    --namespace external-secrets \
    --create-namespace \
    --set installCRDs=true \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=$(aws cloudformation describe-stacks \
        --stack-name ${STACK_NAME_PREFIX}-eks \
        --query 'Stacks[0].Outputs[?OutputKey==`ExternalSecretsOperatorRoleArn`].OutputValue' \
        --output text)
```

## 8. Configure AWS Secrets Manager Access
```bash
# Set role ARN
export SECRETSTORE_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name ${STACK_NAME_PREFIX}-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`SecretStoreRoleArn`].OutputValue' \
    --output text)

# Verify the ARN is assigned to the environment variable
echo $SECRETSTORE_ROLE_ARN

# Create ServiceAccount and SecretStore
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-secrets-sa
  namespace: sdp
  annotations:
    eks.amazonaws.com/role-arn: ${SECRETSTORE_ROLE_ARN}
---
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-backend
  namespace: sdp
spec:
  provider:
    aws:
      service: SecretsManager
      region: ${AWS_REGION}
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa
EOF

# Verify setup
kubectl get secretstore aws-backend -n sdp
```

## 9. Create External Secrets
```bash
kubectl apply -n sdp -f helm/sdp-secrets-${ENVIRONMENT}.yaml
kubectl get externalsecret sdp-secrets -n sdp
```

## 10. Install Nginx Ingress Controller
```bash
# Add and update repository
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm install ingress-nginx ingress-nginx/ingress-nginx \
    --namespace ingress-nginx \
    --version 4.11.0 \
    --create-namespace \
    --set controller.service.type=LoadBalancer \
    --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb \
    --set controller.ingressClassResource.name=ingress-public \
    --set controller.ingressClassResource.enabled=true \
    --set controller.ingressClassResource.default=true \
    --set controller.config.allow-snippet-annotations="true"
```

## 11. Install Cert-Manager
```bash
# Add Jetstack helm repo
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Set role ARN
export CERT_MANAGER_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name ${STACK_NAME_PREFIX}-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`CertManagerRoleArn`].OutputValue' \
    --output text)

# Install cert-manager
helm install cert-manager jetstack/cert-manager \
    --namespace cert-manager \
    --create-namespace \
    --set installCRDs=true \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=$CERT_MANAGER_ROLE_ARN

# Verify installation
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=120s

# Apply ClusterIssuer
kubectl apply -f helm/cluster-issuer.yaml
```

## 12. Install External-DNS
```bash
# Add and update repository
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
helm repo update

# Install external-dns
helm install external-dns external-dns/external-dns \
    --namespace external-dns \
    --create-namespace \
    --set provider=aws \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${ENVIRONMENT}-external-dns-role" \
    --set policy=upsert-only \
    --set "domainFilters[0]=${DOMAIN_NAME}" \
    --set txtOwnerId=eks \
    --set interval=1m

# Verify installation
kubectl get pods -n external-dns
```

## 13. Deploy SDP Helm Chart

### Determine your SDP Domain Structure

The SDP platform requires two base-level domains for frontend and backend access, configured in the Helm chart using Kubernetes route configurations. These domains form the foundation for all tenant-specific URLs:

```yaml
sdp:
  route:
    domain: api.example.org
    mtnDomain: "*.api.example.org"

dashboard:
  route:
    domain: "dashboard.example.org"
    mtnDomain: "*.dashboard.example.org"
```

#### Base Domains Configuration

1. **Frontend (Dashboard)**
   - Base Domain: `dashboard.example.org`
   - Environment Variable: `SDP_UI_BASE_URL`
   - Purpose: Web interface for administrators to manage disbursements

2. **Backend (API)**
   - Base Domain: `api.example.org`
   - Environment Variable: `BASE_URL`
   - Purpose: API access and service-to-service communication

#### Tenant URL Structure

Each tenant gets their own dedicated URLs under the base domains:

```
Dashboard: https://<tenant>.dashboard.example.org
Backend:  https://<tenant>.api.example.org
```

**Example: "ridedash" Tenant**
```
Dashboard: https://ridedash.dashboard.example.org
Backend:  https://ridedash.api.example.org
```

### Add Stellar Repository
```bash
helm repo add stellar https://helm.stellar.org
```

### Configure Domain Structure
The Helm values file (`helm-values-example.yaml`) contains example domain configurations. Before deploying, you should:

1. Review the domain settings in `helm-values-example.yaml`
2. Replace the example domain with your actual domain:
   ```bash
   # Replace example.org with your actual domain
   sed -i '' "s/example.org/${DOMAIN_NAME}/g" helm/helm-values-example.yaml
   ```

### Install SDP
```bash
helm install sdp stellar/stellar-disbursement-platform \
    -f helm/helm-values-example.yaml \
    --namespace sdp
```

### Verify Pods are healthy
```bash
kubectl -n sdp get pods
```

## 14. Adding an SDP Tenant

### Get the SDP Pod name and exec to its shell
```bash
# Get pod name
SDP_POD=$(kubectl -n sdp get pods -l app=sdp -o jsonpath='{.items[0].metadata.name}')

# Port forward to the pod
kubectl -n sdp port-forward pod/${SDP_POD} 8003:8003
```

### Add a tenant using port-forwarding to the /tenants endpoint
You need to use Basic Auth for API requests the the tenant API endpoint. You will first need to port forward to the SDP pod on port 8003.
```bash
kubectl -n sdp get pods                    ⎈ dev-sdp-cluster  12:58:44
NAME                             READY   STATUS    RESTARTS        AGE
sdp-548ccbb67b-gw2tt             1/1     Running   0               8m55s
sdp-ap-58b6cc978-b2wrf           1/1     Running   0               8m55s
sdp-dashboard-848d455d6d-s5b9w   1/1     Running   0               8m55s
sdp-tss-5c4c4847c-wfnnh          1/1     Running   1 (8m50s ago)   8m55s
kubectl -n sdp port-forward pod/sdp-548ccbb67b-gw2tt 8003:8003
```

```bash
echo -n "youremail@yourdomain.org:admin-api-key" | base64                                                                                               ✘ INT  13:13:42
eW91cmVtYWlsQHlvdXJkb21haW4ub3JnOmFkbWluLWFwaS1rZXk=
  ~ ❯ curl --location 'http://localhost:8003/tenants/' \                                                                                                             13:13:50
--header 'Content-Type: application/json' \
--header 'Authorization: Basic eW91cmVtYWlsQHlvdXJkb21haW4ub3JnOmFkbWluLWFwaS1rZXk=' \
--data-raw '{
    "name": "ridedash",
    "owner_email": "owner@ownersemail.org",
    "owner_first_name": "John",
    "owner_last_name": "Doe",
    "organization_name": "ridedash",
    "sdp_ui_base_url": "https://dashboard.mystellarsdpdomain.org",
    "base_url": "https://api.mystellarsdpdomain.org",
    "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
}'
{
  "id": "91bf5cd8-c20f-4754-9447-890f3d166224",
  "name": "ridedash",
  "base_url": "https://api.mystellarsdpdomain.org",
  "sdp_ui_base_url": "https://dashboard.mystellarsdpdomain.org",
  "status": "TENANT_PROVISIONED",
  "is_default": false,
  "created_at": "2025-05-04T17:14:21.089835Z",
  "updated_at": "2025-05-04T17:14:22.563535Z",
  "deleted_at": null,
  "distribution_account_address": "GADPUDSM6OFQRIWPV73KH3LDSLYFG6JDMAVT6B23TW4Z5ORR7RS3LUI2",
  "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
  "distribution_account_status": "ACTIVE"
}%
```

## Cleanup and Teardown
To remove all resources created by this deployment:

```bash
# Delete Helm releases
helm uninstall sdp -n sdp
helm uninstall external-dns -n external-dns
helm uninstall cert-manager -n cert-manager
helm uninstall ingress-nginx -n ingress-nginx
helm uninstall external-secrets -n external-secrets

# Delete namespaces
kubectl delete namespace sdp external-dns cert-manager ingress-nginx external-secrets

# Delete CloudFormation stacks (in reverse order)
aws cloudformation delete-stack --stack-name ${STACK_NAME_PREFIX}-eks --region ${AWS_REGION}
aws cloudformation delete-stack --stack-name ${STACK_NAME_PREFIX}-keys-eks --region ${AWS_REGION}
aws cloudformation delete-stack --stack-name ${STACK_NAME_PREFIX}-database --region ${AWS_REGION}
aws cloudformation delete-stack --stack-name ${STACK_NAME_PREFIX}-network --region ${AWS_REGION}
```

## Troubleshooting

### External Secrets Issues
```bash
# Check OIDC provider configuration
aws iam list-open-id-connect-providers

# Verify ServiceAccount configuration
kubectl describe serviceaccount external-secrets-sa -n sdp

# Force sync ExternalSecret
kubectl annotate externalsecret sdp-secrets -n sdp force-sync=$(date +%s) --overwrite
```

### Database Connectivity Testing
```bash
# Get database endpoint
DB_ENDPOINT=$(aws cloudformation describe-stacks \
    --stack-name ${STACK_NAME_PREFIX}-database \
    --query 'Stacks[0].Outputs[?OutputKey==`DBEndpoint`].OutputValue' \
    --output text)

# Create temporary Postgres pod
kubectl run psql-client --rm -it --image=postgres:15 -- /bin/bash

# Inside the pod, test connection
psql "postgres://$USERNAME:$PASSWORD@${DB_ENDPOINT}:5432/sdp_${ENVIRONMENT}"
```

### Security Groups
```bash
# List node groups
aws eks list-nodegroups --cluster-name $(aws cloudformation describe-stacks \
    --stack-name ${STACK_NAME_PREFIX}-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text)

# View detailed pod logs
kubectl logs -n sdp <pod-name>

# Check pod details
kubectl describe pods -n sdp
```

### Check Secrets in Secrets Manager
```bash
aws secretsmanager list-secrets \
  --filters Key=name-prefix,Values=/sdp/${ENVIRONMENT}
```