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
For testnet, you can auto-generate Stellar secrets using the following command: 

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION}
```
For mainnet (or using pre-created Stellar accounts), you will need to provide (at a minimum)the necessary parameters. Example:
```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=DistributionSeed,ParameterValue=your-distribution-account-secret-key \
    ParameterKey=DistributionPublicKey,ParameterValue=your-distribution-account-public-key \
    ParameterKey=SEP10SigningPrivateKey,ParameterValue=your-sep10-signing-private-key \
    ParameterKey=SEP10SigningPublicKey,ParameterValue=your-sep10-signing-public-key \
    ParameterKey=SecretSep10SigningSeed,ParameterValue=your-secret-sep10-signing-secret-key \
    ParameterKey=ChannelAccountEncryptionPassphrase,ParameterValue=your-channel-encryption-passphrase \
    ParameterKey=DistributionAccountEncryptionPassphrase,ParameterValue=your-distribution-encryption-passphrase
```
for a description of these parameters, please see: [Configuring the SDP](https://developers.stellar.org/platforms/stellar-disbursement-platform/admin-guide/configuring-sdp)

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
Before deploying the Stellar Disbursement Platform helm chart you need to configure the helm values.  Review `values-testnet.yaml` (for Stellar Testnet) or `values-mainnet.yaml` (for Stellar Mainnet)  and substitute the example domain with your own.  For example, you may also want to change the front-end (dashboard) and backend (api) base domains.  See [Stellar Disbursement Platform Domain Structure](#stellar-disbursement-platform-domain-structure) for more information.


### Add Stellar Repository
```bash
helm repo add stellar https://helm.stellar.org
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
You need to use Basic Auth for API requests the the tenant API endpoint. You will first need to port forward to the SDP pod on port 8003. Example:
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
echo -n "admin@example.org:admin-api-key" | base64      1m 27s  14:38:20
YWRtaW5AZXhhbXBsZS5vcmc6YWRtaW4tYXBpLWtleQ==

curl --location 'http://localhost:8003/tenants/' \                                                                                                             13:13:50
--header 'Content-Type: application/json' \
--header 'Authorization: Basic YWRtaW5AZXhhbXBsZS5vcmc6YWRtaW4tYXBpLWtleQ==' \
--data-raw '{
    "name": "ridedash",
    "owner_email": "admin@example.org",
    "owner_first_name": "John",
    "owner_last_name": "Doe",
    "organization_name": "ridedash",
    "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
}'

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

## Additional Information

### Stellar Disbursement Platform Domain Structure
The SDP platform uses two base-level domains for multi-tenant frontend and backend access. For example, lets say your hosted public domain is `api.example.org`. Then, you could configure a subdomain called `api.example.org` as the base-level domain for api access and `dashboard.example.org` as the front-end dashboard base-level domain.   If you then added a tenant (eg `ridedash`) to the SDP, the api and dashboard URLs for them would be `ridedash.api.example.org` and `ridedash.dashboard.example.org` respectively.  you can see this example in the helm-example-values file.   

## Example Helm Values configuration
The following illustrates the example configuration for backend (api) and frontend (dashboard) base domains for the public domain `example.org`. Note, these domains must have a wild-card certificate.
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

The following illustrates the kubernetes configurations that result from the above helm values.
```bash
kubectl -n sdp get ingress              
NAME            CLASS            HOSTS                                           ADDRESS                                                                         PORTS     AGE
sdp             ingress-public   api.example.org,*.api.example.org               a3ca0226bd4494ffb808a64476ddfc4f-66bf685869e3cc2e.elb.us-west-2.amazonaws.com   80, 443   9s
sdp-ap          ingress-public   ap-api.example.org                              a3ca0226bd4494ffb808a64476ddfc4f-66bf685869e3cc2e.elb.us-west-2.amazonaws.com   80, 443   9s
sdp-dashboard   ingress-public   dashboard.example.org,*.dashboard.example.org   a3ca0226bd4494ffb808a64476ddfc4f-66bf685869e3cc2e.elb.us-west-2.amazonaws.com   80, 443   9s

kubectl -n sdp get service                    ⎈ dev-sdp-cluster  14:41:55
NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)             AGE
sdp             ClusterIP   172.20.160.118   <none>        8000/TCP,8003/TCP   2m39s
sdp-ap          ClusterIP   172.20.246.71    <none>        8080/TCP,8085/TCP   2m39s
sdp-dashboard   ClusterIP   172.20.112.135   <none>        80/TCP              2m39s

kubectl -n sdp describe ingress sdp     ✘ INT ⎈ dev-sdp-cluster  14:47:04
Name:             sdp
Labels:           app.kubernetes.io/instance=sdp
                  app.kubernetes.io/managed-by=Helm
                  app.kubernetes.io/name=sdp
                  app.kubernetes.io/version=3.6.2
                  helm.sh/chart=stellar-disbursement-platform-3.6.4
Namespace:        sdp
Address:          a3ca0226bd4494ffb808a64476ddfc4f-66bf685869e3cc2e.elb.us-west-2.amazonaws.com
Ingress Class:    ingress-public
Default backend:  <default>
TLS:
  api-cert terminates api.example.org,*.api.example.org
Rules:
  Host               Path  Backends
  ----               ----  --------
  api.example.org
                     /   sdp:8000 (10.0.2.230:8000)
  *.api.example.org
                     /   sdp:8000 (10.0.2.230:8000)
Annotations:         cert-manager.io/cluster-issuer: letsencrypt-prod
                     meta.helm.sh/release-name: sdp
                     meta.helm.sh/release-namespace: sdp
                     nginx.ingress.kubernetes.io/custom-response-headers:
                       X-Frame-Options: DENY || X-Content-Type-Options: nosniff || Strict-Transport-Security: max-age=31536000; includeSubDomains
                     nginx.ingress.kubernetes.io/limit-burst-multiplier: 5
                     nginx.ingress.kubernetes.io/limit-rpm: 120
Events:
  Type    Reason             Age                    From                       Message
  ----    ------             ----                   ----                       -------
  Normal  CreateCertificate  5m39s                  cert-manager-ingress-shim  Successfully created Certificate "api-cert"
  Normal  Sync               5m31s (x2 over 5m39s)  nginx-ingress-controller   Scheduled for sync

  kubectl -n sdp describe ingress sdp-dashboard
Name:             sdp-dashboard
Labels:           app.kubernetes.io/instance=sdp-dashboard
                  app.kubernetes.io/managed-by=Helm
                  app.kubernetes.io/name=sdp-dashboard
                  app.kubernetes.io/version=3.6.2
                  helm.sh/chart=stellar-disbursement-platform-3.6.4
Namespace:        sdp
Address:          a3ca0226bd4494ffb808a64476ddfc4f-66bf685869e3cc2e.elb.us-west-2.amazonaws.com
Ingress Class:    ingress-public
Default backend:  <default>
TLS:
  sdp-dashboard-cert terminates dashboard.example.org,*.dashboard.example.org
Rules:
  Host                     Path  Backends
  ----                     ----  --------
  dashboard.example.org
                           /   sdp-dashboard:80 (10.0.2.248:80)
  *.dashboard.example.org
                           /   sdp-dashboard:80 (10.0.2.248:80)
Annotations:               cert-manager.io/cluster-issuer: letsencrypt-prod
                           meta.helm.sh/release-name: sdp
                           meta.helm.sh/release-namespace: sdp
Events:
  Type    Reason             Age                   From                       Message
  ----    ------             ----                  ----                       -------
  Normal  CreateCertificate  6m6s                  cert-manager-ingress-shim  Successfully created Certificate "sdp-dashboard-cert"
  Normal  Sync               5m58s (x2 over 6m6s)  nginx-ingress-controller   Scheduled for sync

```

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