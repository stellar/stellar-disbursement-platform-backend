# # Stellar Disbursement Platform (SDP) Kubernetes Deployment Guide

## Prerequisites
- AWS CLI installed and configured
- Helm installed
- kubectl configured to connect to your cluster

## Cloudformation Stacks
This guide walks through deploying the Stellar Disbursement Platform (SDP) infrastructure on AWS. The deployment consists of four CloudFormation stacks that create the necessary infrastructure in a specific order:

- Network Stack (sdp-network-eks.yaml)
  - Creates or uses existing VPC and subnets Sets up networking for both public and private resources. Exports used (imported) by database and EKS stack to deploy resources.
- Database Stack (sdp-database-eks.yaml)
  - Deploys RDS PostgreSQL database in private subnet
  - creates necessary database secrets in AWS Secrets Manager
- Keys Stack (sdp-keys.yaml) 
  - Manages Stellar keys and encryption secrets
  - Provide keys as parameters or leave blank to auto-generate
  - Stores all secrets in AWS Secrets Manager
- EKS Stack (sdp-eks.yaml)
  - Creates EKS cluster and node group
  - Sets up IAM roles and security groups
  - Configures necessary permissions for Kubernetes services
After the CloudFormation stacks are deployed, additional Kubernetes resources are installed via Helm charts to complete the setup.

##Verify AWS CLI Configuration
```bash
aws configure list
aws sts get-caller-identity
```

## 1. Network Stack Deployment
Deploy the networking infrastructure:

```bash
aws cloudformation create-stack \
  --stack-name sdp-network \
  --template-body file://sdp-network-eks.yaml \
  --parameters \
    ParameterKey=env,ParameterValue=dev \
    ParameterKey=AWSRegion,ParameterValue=us-west-2 \
    ParameterKey=ExistingVPCId,ParameterValue="" \
    ParameterKey=VPCCidr,ParameterValue="10.0.0.0/16"
```

**Note**: To use existing network resources, provide the VPC and subnet IDs:
```bash
aws cloudformation create-stack \
  --stack-name sdp-network \
  --template-body file://sdp-network-eks.yaml \
  --parameters \
    ParameterKey=env,ParameterValue=dev \
    ParameterKey=ExistingVPCId,ParameterValue=vpc-1234567890abcdef0 \
    ParameterKey=ExistingPublicSubnet1Id,ParameterValue=subnet-xxxxx \
    ParameterKey=ExistingPublicSubnet2Id,ParameterValue=subnet-yyyyy \
    ParameterKey=ExistingPrivateSubnet1Id,ParameterValue=subnet-aaaaa \
    ParameterKey=ExistingPrivateSubnet2Id,ParameterValue=subnet-bbbbb
```

Wait for stack completion:
```bash
aws cloudformation wait stack-create-complete --stack-name sdp-network
```

## 2. Database Stack Deployment
Deploy the RDS database:

```bash
aws cloudformation create-stack \
  --stack-name sdp-database \
  --template-body file://sdp-database-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    ParameterKey=env,ParameterValue=dev \
    ParameterKey=NetworkStackName,ParameterValue=sdp-network \
    ParameterKey=DBInstanceClass,ParameterValue=db.t3.small \
    ParameterKey=DBUsername,ParameterValue=postgres \
    ParameterKey=DBPassword,ParameterValue=your-secure-password \
    ParameterKey=MultiAZ,ParameterValue=false
```

Wait for stack completion:
```bash
aws cloudformation wait stack-create-complete --stack-name sdp-database
```

## 3. Keys Stack Deployment
Deploy the secrets and keys management stack:

```bash
aws cloudformation create-stack \
  --stack-name sdp-keys \
  --template-body file://sdp-keys.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    ParameterKey=env,ParameterValue=dev \
    ParameterKey=namespace,ParameterValue=sdp
```

**Note**: Leave the following parameters empty to auto-generate new keys:
- DistributionSeed
- DistributionPublicKey
- SEP10SigningPrivateKey
- SEP10SigningPublicKey
- ChannelAccountEncryptionPassphrase
- DistributionAccountEncryptionPassphrase

Wait for stack completion:
```bash
aws cloudformation wait stack-create-complete --stack-name sdp-keys
```

## 4. EKS Cluster Deployment
Deploy the EKS cluster:

```bash
aws cloudformation create-stack \
  --stack-name sdp-eks \
  --template-body file://sdp-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameters \
    ParameterKey=env,ParameterValue=dev \
    ParameterKey=NetworkStackName,ParameterValue=sdp-network \
    ParameterKey=DatabaseStackName,ParameterValue=sdp-database \
    ParameterKey=KeysStackName,ParameterValue=sdp-keys
```

Wait for stack completion (this will take ~15-20 minutes):
```bash
aws cloudformation wait stack-create-complete --stack-name sdp-eks
```

## 5. Configure kubectl
After the EKS cluster is created, configure kubectl:

```bash
aws eks update-kubeconfig --name $(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text) \
    --region your-region
```

## 6. Follow Manual Helm Deployment Steps
Continue with the manual Helm deployment steps as provided in the deployment guide, which includes:
1. External Secrets Operator installation
2. AWS Secrets Manager access configuration
3. External Secrets creation
4. Nginx Ingress Controller installation
5. Cert-Manager installation
6. External-DNS setup
7. SDP Helm chart deployment

Refer to the Helm deployment instructions for these steps.

## Verification Steps

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

### Check Secrets in Secrets Manager
```bash
aws secretsmanager list-secrets \
  --filters Key=name-prefix,Values=/sdp/dev
```


## Initial Setup
```bash
# Configure kubectl for your cluster
aws eks update-kubeconfig --name $(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`ClusterName`].OutputValue' \
    --output text)

# Create namespace
kubectl create namespace sdp
```

## 1. External Secrets Operator Installation
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

# Verify installation
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=external-secrets -n external-secrets --timeout=120s
```

## 2. Configure AWS Secrets Manager Access
```bash
# Set role ARN
export SECRETSTORE_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name sdp-eks \
    --query 'Stacks[0].Outputs[?OutputKey==`SecretStoreRoleArn`].OutputValue' \
    --output text)

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
```

## 3. Create External Secrets
```bash
kubectl apply -n sdp -f eks/helm/sdp-secrets.yaml
kubectl get externalsecret sdp-secrets -n sdp
```

## 4. Install Nginx Ingress Controller
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

## 5. Install Cert-Manager
```bash
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
kubectl apply -f eks-helm/cluster-issuer.yaml
```

## 6. Install External-DNS
```bash
# Add and update repository
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
helm repo update

# Install external-dns
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
helm install external-dns external-dns/external-dns \
    --namespace external-dns \
    --create-namespace \
    --set provider=aws \
    --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::${AWS_ACCOUNT_ID}:role/dev-external-dns-role" \
    --set policy=sync \
    --set "domainFilters[0]=mystellarsdpdomain.org" \
    --set txtOwnerId=eks \
    --set interval=1m

# Verify installation
kubectl get pods -n external-dns
```

## 7. Deploy SDP Helm Chart
```bash
# Add Stellar repository
helm repo add stellar https://helm.stellar.org

# Install SDP
helm install sdp stellar/stellar-disbursement-platform \
    -f eks-helm/values.yaml \
    --namespace sdp

# Verify deployment
kubectl -n sdp get pods
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
