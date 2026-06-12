# Stellar Disbursement Platform (SDP) AWS Kubernetes (EKS) Deployment Guide

## Prerequisites
- AWS CLI installed and configured
- Helm installed
- kubectl configured to connect to your cluster
- A Route53 public hosted zone for your domain 

## Environment Setup
Before starting, set these environment variables:
```bash
# Required variables
export AWS_REGION=your-region  # e.g., us-west-2, eu-west-1, etc.
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export ENVIRONMENT=dev  # or prod, staging, etc.
export NAMESPACE=sdp

# Optional variables (for customizing deployment)
export STACK_NAME_PREFIX=sdp  # Prefix for all CloudFormation stacks
export DOMAIN_NAME=example.org  # Your registered domain
```

## Cloudformation Stacks
This guide walks through deploying the Stellar Disbursement Platform (SDP) infrastructure on AWS. The deployment consists of four CloudFormation stacks that create the necessary infrastructure in a specific order:

- Network Stack (`sdp-network-eks.yaml`)
  - Creates or uses existing VPC and subnets
  - Sets up networking for both public and private resources
  - Exports used (imported) by database and EKS stack to deploy resources

- Database Stack (`sdp-database-eks.yaml`)
  - Deploys RDS PostgreSQL database in private subnet
  - Creates necessary database secrets in AWS Secrets Manager

- Keys Stack (`sdp-keys-eks.yaml`) [Optional]
  - Manages Stellar and encryption keys by either:
    - Using provided keys via parameters, or
    - Auto-generating keys using Lambda function for dev/test environments
  - Stores all keys and secrets in AWS Secrets Manager under `/${namespace}/${ENVIRONMENT}/SECRET_NAME`
  - Keys include SEP-10 signing keys, distribution account keys, etc.

- EKS Stack (`sdp-eks.yaml`)
  - Creates EKS cluster and node group
  - Sets up IAM roles and security groups
  - Configures IRSA (IAM Roles for Service Accounts)
  - Sets up permissions for pods to access secrets stored in AWS Secrets Manager

After the CloudFormation stacks are deployed, additional Kubernetes resources are installed via Helm charts to complete the setup. The SDP expects secrets to be available as Kubernetes secrets, but how those secrets are synchronized (whether through ExternalSecrets, direct creation, or other means) is left to the deployer's preference.

> **Note**: Create each successive stack only after the previous one has finished. You can check if stack creation is complete with the following command:
 ```bash
  aws cloudformation describe-stacks --region $AWS_REGION \
    --query "Stacks[?contains(StackName,'${STACK_NAME_PREFIX}-')].[StackName,StackStatus]" \
    --output table
  ```

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
Deploy the networking infrastructure. 

Review custom parameters if needed, such as if your deployment will use an existing VPC. By default, a new one will be created with no additional parameters necessary.

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-network \
  --template-body file://sdp-network-eks.yaml \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=env,ParameterValue=${ENVIRONMENT}
```

## 2. Database Stack Deployment
Deploy the RDS database. 

Review custom parameters if needed. Notable custom parameter(s) to override:
| ParameterKey | Default Value | Description |
| --- | --- | --- |
| `DBPassword` | `postgres` | Database admin password. Be sure to change this for production deployments. |
| `MultiAZ` | `false` | When `true`, provisions a replica of your RDS instance in a second availability zone (a physically separate data center) in case of an outage / failure of the primary. Roughly doubles the cost of the RDS instance |
| `DeletionProtection` | `false` | When `true`, blocks deletion of the RDS instance by anyone with the required IAM permissions until the protection is explicitly disabled.  |

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-database \
  --template-body file://sdp-database-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=NetworkStackName,ParameterValue=${STACK_NAME_PREFIX}-network \
    ParameterKey=env,ParameterValue=${ENVIRONMENT} \
    ParameterKey=namespace,ParameterValue=${NAMESPACE} \
```

## 3. Keys Stack Deployment

Create and store SDP secrets in AWS Secrets Manager. The `sdp-keys-eks.yaml` file contains parameter fields for core and optional secrets that need to be provided. All secrets are stored at `/${namespace}/${ENVIRONMENT}/SECRET_NAME` (default `namespace` = `sdp`) and synced to Kubernetes via ExternalSecrets. 

### 3.1 Testnet Secrets

For **testnet** / dev environments, if no parameter values are supplied, core secret default values will be used. Stellar keys and passphrases need to be auto-generated if not supplied (see `sdp-keys-eks.yaml`). You can deploy a lambda function that generates Stellar keypairs and funds the distribution account via friendbot.

This must be built and uploaded before the main keys stack:

```bash
# Create a bucket to hold the layer (name must be globally unique)
aws s3 mb s3://your-stellar-layer-bucket --region ${AWS_REGION}

# Build node_modules and zip it (requires Node 22 installed on machine)
mkdir -p nodejs &&
cd nodejs && npm install @stellar/stellar-sdk && cd .. &&
zip -r stellar-layer.zip nodejs/

# Upload it to the bucket
aws s3 cp stellar-layer.zip s3://your-stellar-layer-bucket/stellar-layer.zip
```

Then pass the bucket name when deploying the stack below via `ParameterKey=StellarLayerS3Bucket ParameterValue=your-stellar-layer-bucket`. This parameter is required whenever any Stellar key or passphrase is left blank for auto-generation, and the parameter value must be the bucket name you provided above (e.g. `your-stellar-layer-bucket`).

> **Note**: Do not generate these keypairs for mainnet/prod. Skip this section and bring your own keys.

Create the SDP secrets for testnet using the following command:

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=StellarLayerS3Bucket,ParameterValue=your-stellar-layer-bucket
```

### 3.2 Mainnet Secrets

For **mainnet** (or when using pre-created Stellar accounts), you will need to provide the necessary parameter values, both for core and any additional optional secrets. Core secrets have insecure defaults that **must** be overriden for mainnet deployment. 

Please review optional secrets as desired. For a description of these, please see: [Configuring the SDP](https://developers.stellar.org/docs/platforms/stellar-disbursement-platform/admin-guide/configuring-sdp) and the [SDP Helm Chart README](https://github.com/stellar/stellar-disbursement-platform-backend/blob/develop/helmchart/sdp/README.md).

Once parameters have been configured, you can create the SDP secrets using the following command (incomplete example, supply any missing parameters):

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters \
    ParameterKey=env,ParameterValue=${ENVIRONMENT} \
    ParameterKey=namespace,ParameterValue=${NAMESPACE} \
    ParameterKey=DistributionSeed,ParameterValue=your-distribution-account-secret-key \
    ParameterKey=DistributionPublicKey,ParameterValue=your-distribution-account-public-key \
    ParameterKey=Sep10SigningPrivateKey,ParameterValue=your-sep10-signing-private-key \
    ParameterKey=Sep10SigningPublicKey,ParameterValue=your-sep10-signing-public-key \
    # etc
```
Alternatively, you can write all the parameters as key-value pairs in a JSON file (incomplete example):

```json
[
  {
    "ParameterKey": "env",
    "ParameterValue": "prod"
  },
  {
    "ParameterKey": "DistributionSeed",
    "ParameterValue": "your-distribution-account-secret-key"
  }
]
```
Then pass this JSON file to the `create-stack` command:

```bash
aws cloudformation create-stack \
  --stack-name ${STACK_NAME_PREFIX}-keys-eks \
  --template-body file://sdp-keys-eks.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ${AWS_REGION} \
  --parameters file://params.json
```

> **Security Notice**: Secrets passed as CloudFormation parameters are stored in the stack's parameter history and can be retrieved in plaintext by anyone with Cloudformation read permissions (`cloudformation:DescribeStacks`) in your AWS account. For production deployments where Secrets Manager access is restricted separately (`secretsmanager:GetSecretValue`), consider pre-creating sensitive values directly in AWS Secrets Manager instead of deploying this stack and passing them as parameters.

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
    ParameterKey=DatabaseStackName,ParameterValue=${STACK_NAME_PREFIX}-database \
    ParameterKey=env,ParameterValue=${ENVIRONMENT} \
    ParameterKey=namespace,ParameterValue=${NAMESPACE}
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

> **Note**: If deploying under a different namespace, replace sdp in the namespace-scoped kubectl/helm commands.

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
apiVersion: external-secrets.io/v1
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
envsubst '${NAMESPACE} ${ENVIRONMENT}' < helm/sdp-secrets.yaml | kubectl apply -f -
kubectl get externalsecret sdp-secrets -n ${NAMESPACE}
```

## 10. Install Nginx Ingress Controller
```bash
# Add and update repository
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm install ingress-nginx ingress-nginx/ingress-nginx \
    --namespace ingress-nginx \
    --version 4.15.1 \
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
export EMAIL=your-email.org
envsubst '${AWS_REGION} ${EMAIL}' < helm/cluster-issuer.yaml | kubectl apply -f -
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

> Note: Cert-manager (Step 11) and External-DNS (Step 12) both manage DNS through **Route53**, so `${DOMAIN_NAME}` must be served by a Route53 **public hosted zone** before this step. If it isn't, create one (or delegate a subdomain to one), repoint your registrar's `NS` records at it, and let the delegation propagate before proceeding to Step 13.

## 13. Deploy SDP Helm Chart
Before deploying the Stellar Disbursement Platform helm chart you need to configure the helm values.  Review `values-testnet.yaml` (for Stellar Testnet) or `values-mainnet.yaml` (for Stellar Mainnet)  and substitute the default values with your own. For example, you may also want to change the front-end (dashboard) and backend (api) base domains (See [Stellar Disbursement Platform Domain Structure](#stellar-disbursement-platform-domain-structure) for more information). You should also add any additional values you require, such as embedded wallet or AWS/Twilio messaging support.

See the following to learn about SDP helm values: [Configuring the SDP](https://developers.stellar.org/docs/platforms/stellar-disbursement-platform/admin-guide/configuring-sdp) and the [SDP Helm Chart README](https://github.com/stellar/stellar-disbursement-platform-backend/blob/develop/helmchart/sdp/README.md).


### Install the SDP Chart

The chart can be installed either from a packaged chart or directly from the git repository.

### From a packaged chart

```shell
# Add the Stellar Helm repository to Helm
helm repo add stellar https://helm.stellar.org/charts
```

```shell
# Install the chart
# Replace helm-values-example.yaml with the actual path to values-testnet.yaml or values-mainnet.yaml.
helm install sdp stellar/stellar-disbursement-platform \
    -f helm/helm-values-example.yaml \
    --namespace sdp
```

### From the git repository

```shell
# Clone the git repository
git clone git@github.com:stellar/stellar-disbursement-platform-backend.git
```

```shell
# Change directory to the helm chart
cd stellar-disbursement-platform-backend/helmchart/sdp
```

```shell
# Install the chart
# Replace helm-values-example.yaml with the actual path to values-testnet.yaml or values-mainnet.yaml. It will normally be in a different directory.
helm install sdp \
  -f helm-values-example.yaml . \
  --namespace sdp
```

### Verify Pods are healthy
```bash
kubectl -n sdp get pods --show-labels
```

## 14. Adding an SDP Tenant

### Get the SDP Pod name and exec to its shell
```bash
# Get pod name
SDP_POD=$(kubectl -n sdp get pods -l app.kubernetes.io/name=sdp -o jsonpath='{.items[0].metadata.name}')
echo $SDP_POD

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
# Derive auth header by typing admin credentials
AUTH_HEADER=$(echo -n "admin@example.org:admin-api-key" | base64 -w 0)     

# Or retrieve the credentials from secrets
ADMIN_ACCOUNT=$(kubectl get secret --namespace sdp sdp-secrets -o jsonpath="{.data.ADMIN_ACCOUNT}" | base64 --decode)
ADMIN_API_KEY=$(kubectl get secret --namespace sdp sdp-secrets -o jsonpath="{.data.ADMIN_API_KEY}" | base64 --decode)
AUTH_HEADER=$(echo -n "$ADMIN_ACCOUNT:$ADMIN_API_KEY" | base64 -w 0)

# Command to add tenant.
curl --location 'http://localhost:8003/tenants/' \
--header 'Content-Type: application/json' \
--header "Authorization: Basic $AUTH_HEADER" \
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
