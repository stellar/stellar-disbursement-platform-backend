# Stellar Disbursement Platform (SDP) AWS Kubernetes (EKS) Deployment Guide

## Prerequisites
- AWS CLI installed and configured
- Helm installed
- kubectl configured to connect to your cluster

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
  - Stores all keys and secrets in AWS Secrets Manager under /sdp/${env}/ path
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
  --stack-name sdp-network \
  --template-body file://sdp-network-eks.yaml \
```

## 2. Database Stack Deployment
Deploy the RDS database. Review custom parameters if needed.

```bash
aws cloudformation create-stack \
  --stack-name sdp-database \
  --template-body file://sdp-database-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    ParameterKey=NetworkStackName,ParameterValue=sdp-network
```

## 3. Keys Stack Deployment
Deploy the secrets and keys management stack:

```bash
aws cloudformation create-stack \
  --stack-name sdp-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM 
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
  --stack-name sdp-eks \
  --template-body file://sdp-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    ParameterKey=NetworkStackName,ParameterValue=sdp-network \
    ParameterKey=DatabaseStackName,ParameterValue=sdp-database
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
aws eks update-kubeconfig --name $(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text) \
    --region your-region
```

verify you are pointing kubectl to the correct AWS EKS Cluster 
```bash
kubectl config get-contexts
```

## 6. Create Namespace

```bash
# Create namespace
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
        --stack-name sdp-eks \
        --query 'Stacks[0].Outputs[?OutputKey==`ExternalSecretsOperatorRoleArn`].OutputValue' \
        --output text)

## 8. Configure AWS Secrets Manager Access
```bash
# Set role ARN
export SECRETSTORE_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
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
      region: us-west-2
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa
EOF

# Verify setup
kubectl get secretstore aws-backend -n sdp

# You should see output similar to the following with READY state True:
#NAME          AGE   STATUS   CAPABILITIES   READY
#aws-backend   7s    Valid    ReadWrite      True
```

## 9. Create External Secrets
```bash
kubectl apply -n sdp -f helm/sdp-secrets-dev.yaml
kubectl get externalsecret sdp-secrets -n sdp

# Verify. You should see STATUS: SecretSynced and READY: True
#NAME          STORETYPE     STORE         REFRESH INTERVAL   STATUS         READY
#sdp-secrets   SecretStore   aws-backend   1h                 SecretSynced   True

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


# Verify installation
kubectl get pods -n ingress-nginx
```

## 11. Install Cert-Manager
```bash
# Add Jetstack helm repo
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Set role ARN
export CERT_MANAGER_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
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
Replace the domainFilters with your registered domain below.

```bashd
# Add and update repository
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
helm repo update

# Install external-dns with UPSERT policy and placeholder domain. Be sure to repl
helm install external-dns external-dns/external-dns \
    --namespace external-dns \
    --create-namespace \
    --set provider=aws \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::${AWS_ACCOUNT_ID}:role/dev-external-dns-role" \
    --set policy=upsert-only \
    --set "domainFilters[0]=<your-domain-here>" \
    --set txtOwnerId=eks \
    --set interval=1m

# Verify installation
kubectl get pods -n external-dns
```

## 13. Deploy SDP Helm Chart

### Add Stellar Repository
```bash
helm repo add stellar https://helm.stellar.org
```

### Modify Values File
Replace occurrences of "mystellarsdpdomain.org" in [values-dev.yaml](aws/cloudformation/values-dev.yaml) with your registered domain.

### Install SDP

```bash
helm install sdp stellar/stellar-disbursement-platform -f helm/values-dev.yaml --namespace sdp
```
### Verify Pods are healthy 
```
kubectl -n sdp get pods
```

## 14. Adding an SDP Tenant 

### Get the SDP Pod name and exec to its shell
```bash
kubectl -n sdp get pods                                                                                                   ✔  11:30:38
NAME                             READY   STATUS    RESTARTS   AGE
sdp-5bddb74b4d-skqsv             1/1     Running   0          13m
sdp-ap-58b6cc978-8q4sg           1/1     Running   0          13m
sdp-dashboard-7799755844-nwnw5   1/1     Running   0          13m
sdp-tss-84bc97659-bf9pb          1/1     Running   0          13m

kubectl -n sdp port-forward pod/sdp-5bddb74b4d-skqsv 8003:8003
```
#### Add a tenant using port-forwarding to the /tenants endpoint 
```bash
curl --location 'http://localhost:8003/tenants/' \
--header 'Content-Type: application/json' \
--header 'Authorization: Basic cmVlY2VAc3RlbGxhci5vcmc6YWRtaW4tYXBpLWtleQ==' \
--data-raw '{
    "name": "ridedash",
    "owner_email": "reece@stellar.org",
    "owner_first_name": "reece",
    "owner_last_name": "markowsky",
    "organization_name": "aid-retreat",
    "sdp_ui_base_url": "https://ridedash.<your-domain>",
    "base_url": "https://ridedash.<your-domain>",
    "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
}'

#example output
{
  "id": "c68146db-5f12-4422-9313-37027ba438bb",
  "name": "ridedash",
  "base_url": "https://ridedash.mystellarsdpdomain.org",
  "sdp_ui_base_url": "https://ridedash.mystellarsdpdomain.org",
  "status": "TENANT_PROVISIONED",
  "is_default": false,
  "created_at": "2025-04-15T19:48:40.029056Z",
  "updated_at": "2025-04-15T19:48:41.790493Z",
  "deleted_at": null,
  "distribution_account_address": "GC4YTNYK3YIRSLYYH7D6R27OEKMOXZYFSRVYDOAQN5NY5SZMIQWJSZ7Q",
  "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
  "distribution_account_status": "ACTIVE"
}
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
# Create temporary Postgres pod
kubectl run psql-client --rm -it --image=postgres:15 -- /bin/bash

# Inside the pod, test connection
psql "postgres://$USERNAME:$PASSWORD@sdp-database-postgresinstance-mls97rxivoee.c76www468ao3.us-west-2.rds.amazonaws.com:5432/sdp_dev"
```

### Security Groups
```bash
# List node groups
aws eks list-nodegroups --cluster-name dev-sdp-cluster

# View detailed pod logs
kubectl logs -n sdp <pod-name>

# Check pod details
kubectl describe pods -n sdp
```

### Check Secrets in Secrets Manager
```bash
aws secretsmanager list-secrets \
  --filters Key=name-prefix,Values=/sdp/dev
```

### Check Network Stack
```bash
aws cloudformation describe-stacks --stack-name sdp-network \
  --query 'Stacks[0].Outputs'
```

### Check Database Connectivity
```bash
aws cloudformation describe-stacks --stack-name sdp-database \
  --query 'Stacks[0].Outputs'
```

### Verify EKS Cluster
```bash
aws eks describe-cluster \
  --name $(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text)
```
