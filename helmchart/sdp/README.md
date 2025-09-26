# Stellar Disbursement Platform Helm Chart

## Table of Contents

- [Introduction](#introduction)
- [Installing the Chart](#installing-the-chart)
  - [From a packaged chart](#from-a-packaged-chart)
  - [From the git repository](#from-the-git-repository)
  - [Prerequisites](#prerequisites)
- [Uninstalling the Chart](#uninstalling-the-chart)
- [Local Development](#local-development)
  - [Prerequisites](#prerequisites-1)
  - [Running the SDP locally](#running-the-sdp-locally)
- [Parameters](#parameters)
  - [Global parameters](#global-parameters)
  - [Stellar Disbursement Platform (SDP) parameters](#stellar-disbursement-platform-sdp-parameters)
  - [Anchor Platform](#anchor-platform)
  - [Transaction Submission Service](#transaction-submission-service)
  - [Dashboard](#dashboard)

## Introduction
This chart bootstraps a Stellar Disbursement Platform (SDP) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

The SDP is a set of services that enable organizations to disburse funds to recipients using the Stellar network. The SDP consists of the following services:
- Stellar Disbursement Platform (SDP) Core Service: the core backend service that performs several functions.
- Anchor Platform: the API server that the wallet uses to authenticate and initiate the recipient’s registration process through the SEP-24 deposit flow.
- Transaction Submission Service (TSS): the service that submits all payment transactions to the Stellar network.
- Dashboard: the user interface administrators use to initiate and track the progress of disbursements.

## Installing the Chart

The chart can be installed either from a packaged chart or directly from the git repository.

### Prerequisites
- Kubernetes 1.19+
- Helm 3.2.0+
- Postgres 14.0+ database deployed in the same Kubernetes cluster
- Kafka (optional) needed for inter-service communication when `eventBroker.type` is set to "KAFKA"

### From a packaged chart

- Add the Stellar Helm repository to Helm
```shell
helm repo add stellar https://helm.stellar.org/charts
```

- Customize the chart by downloading and modifying `minimal-values.yaml`. This chart contains the minimum set of values required to deploy the SDP. For a complete list of values, refer to the [Parameters](#parameters) section below.
```shell
curl -LJO https://raw.githubusercontent.com/stellar/stellar-disbursement-platform-backend/main/helmchart/sdp/minimal-values.yaml
```

- Install the chart
```shell
helm install sdp -f myvalues.yaml stellar/stellar-disbursement-platform
```

### From the git repository

- Clone the git repository
```shell
git clone git@github.com:stellar/stellar-disbursement-platform-backend.git
```

- Change directory to the helm chart
```shell
cd stellar-disbursement-platform-backend/helmchart/sdp
```

- Prepare the values needed to run the SDP locally. We will only need a distribution account and a SEP-10 account. 
Both can be created using the [Stellar Laboratory](https://lab.stellar.org/account/create?$=network$id=testnet&label=Testnet&horizonUrl=https:////horizon-testnet.stellar.org&rpcUrl=https:////soroban-testnet.stellar.org&passphrase=Test%20SDF%20Network%20/;%20September%202015;;).

- Install the chart
It is possible to use the `minimal-values.yaml` file provided in the repository or create your own values file. The `minimal-values.yaml` file contains the minimum set of values required to deploy the SDP.
```shell
helm install sdp -f minimal-values.yaml . \
     --set "global.distributionPublicKey=GCUD...EZW7" \
     --set "global.distributionPrivateKey=SC7G...AGJA" \
     --set "global.sep10PublicKey=GD4L...LYR2U" \
     --set "global.sep10PrivateKey=SBNY...FZAG"
```

## Uninstalling the Chart

To uninstall/delete the `sdp` deployment:

```shell
helm delete sdp
```

## Local Development
Running the SDP locally using the helm chart is easy. We will use the `minikube` tool to run a local Kubernetes cluster.

### Prerequisites
- [minikube](https://minikube.sigs.k8s.io/docs/start/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [helm](https://helm.sh/docs/intro/install/)

### Running the SDP locally
1. Start minikube
```shell
minikube start
```

2. Enable the ingress addon
```shell
minikube addons enable ingress
```

3. Prepare the values needed to run the SDP locally. 
We will only need a distribution account and a SEP-10 account. Both can be created using the [Stellar Laboratory](https://lab.stellar.org/account/create?$=network$id=testnet&label=Testnet&horizonUrl=https:////horizon-testnet.stellar.org&rpcUrl=https:////soroban-testnet.stellar.org&passphrase=Test%20SDF%20Network%20/;%20September%202015;;).

4. Run helm install using the provided `minimal-values.yaml` file
```shell
helm install sdp -f minimal-values.yaml . \
     --set "global.distributionPublicKey=GCUD...EZW7" \
     --set "global.distributionPrivateKey=SC7G...AGJA" \
     --set "global.sep10PublicKey=GD4L...LYR2U" \
     --set "global.sep10PrivateKey=SBNY...FZAG"
```

5. Setup Local DNS resolution.
Add entries to your `/etc/hosts` file to access the services.
```shell
sudo bash -c 'echo "127.0.0.1 dashboard.local sdp.local ap.local admin.local" >> /etc/hosts'
```
Run the following command to enable the minikube tunnel. Make sure to keep this command running in a separate terminal.
```shell
minikube tunnel
```

6. Access the services
With the tunnel running, you can access the services using the following URLs:
- Dashboard: [https://dashboard.local](https://dashboard.local)
- SDP Backend: [https://sdp.local](https://sdp.local)
- SDP Admin API: [https://sdp.local:8003](https://sdp.local:8003)
- Anchor Platform: [https://ap.local](https://ap.local)

## Parameters

### Global parameters

These parameters are shared by all charts.

| Name                                                   | Description                                                                                                                         | Value                                      |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| `global.isPubnet`                                      | Determines if the network is public. Set this to true for public networks.                                                          | `false`                                    |
| `global.replicaCount`                                  | Number of replicas for the application.                                                                                             | `1`                                        |
| `global.resources`                                     | Resource limits and requests for the application pods.                                                                              | `{}`                                       |
| `global.service.type`                                  | Kubernetes Service type for the application.                                                                                        | `ClusterIP`                                |
| `global.autoscaling`                                   | Configuration related to the horizontal pod autoscaling of the application.                                                         |                                            |
| `global.autoscaling.enabled`                           | Determines if autoscaling is enabled for the application.                                                                           | `false`                                    |
| `global.autoscaling.minReplicas`                       | Minimum number of replicas when autoscaling is enabled.                                                                             | `1`                                        |
| `global.autoscaling.maxReplicas`                       | Maximum number of replicas when autoscaling is enabled.                                                                             | `4`                                        |
| `global.autoscaling.targetCPUUtilizationPercentage`    | Target CPU utilization percentage for autoscaling.                                                                                  | `80`                                       |
| `global.autoscaling.targetMemoryUtilizationPercentage` | Target memory utilization percentage for autoscaling.                                                                               | `80`                                       |
| `global.serviceAccount`                                | Configuration related to the Kubernetes Service Account used by the application.                                                    |                                            |
| `global.serviceAccount.create`                         | Determines if a new service account should be created.                                                                              | `false`                                    |
| `global.serviceAccount.annotations`                    | Annotations to be added to the service account.                                                                                     | `nil`                                      |
| `global.serviceAccount.name`                           | Name of the service account to be used. If not set and create is set to true, a name will be generated using the fullname template. | `""`                                       |
| `global.deployment`                                    | Configuration related to the deployment of the application.                                                                         |                                            |
| `global.deployment.nodeSelector`                       | Node selector to determine which nodes should run the pods.                                                                         | `{}`                                       |
| `global.deployment.tolerations`                        | Tolerations to ensure pods aren't scheduled on unsuitable nodes.                                                                    | `[]`                                       |
| `global.deployment.affinity`                           | Affinity rules to determine where pods get scheduled based on node conditions.                                                      | `{}`                                       |
| `global.deployment.priorityClassName`                  | Name of the priority class to be used by the deployment.                                                                            | `""`                                       |
| `global.deployment.topologySpreadConstraints`          | Pod topology spread constraints for all services.                                                                                   | `[]`                                       |
| `global.ephemeralDatabase`                             | Enables or disables the creation of an ephemeral database for testing purposes.                                                     | `true`                                     |
| `global.autoGenerateSecrets`                           | Determines if secrets should be auto-generated.                                                                                     | `false`                                    |
| `global.eventBroker`                                   | Configuration related to the event broker used by the application.                                                                  |                                            |
| `global.eventBroker.type`                              | The type of event broker to be used. Options: "NONE", "KAFKA". Default: "KAFKA".                                                    | `SCHEDULER`                                |
| `global.eventBroker.urls`                              | A comma-separated list of broker URLs for the event broker.                                                                         | `nil`                                      |
| `global.eventBroker.consumerGroupId`                   | The consumer group ID for the event broker.                                                                                         | `nil`                                      |
| `global.eventBroker.kafka`                             | Configuration related to the Kafka event broker.                                                                                    |                                            |
| `global.eventBroker.kafka.securityProtocol`            | The security protocol to be used for the Kafka broker. Options: "PLAINTEXT", "SASL_SSL", "SASL_PLAINTEXT", "SSL".                   | `nil`                                      |
| `global.singleTenantMode`                              | Determines if the SDP service is running in single-tenant mode.                                                                     | `false`                                    |
| `global.distributionPublicKey`                         | The public key of the HOST's Stellar distribution account, used to create channel accounts.                                         | `nil`                                      |
| `global.distributionPrivateKey`                        | The private key of the root Stellar distribution account                                                                            | `nil`                                      |
| `global.sep10PublicKey`                                | Anchor platform SEP10 signing public key.                                                                                           | `nil`                                      |
| `global.sep10PrivateKey`                               | The public key of the Stellar account that signs the SEP-10 transactions. It's also used to sign URLs.                              | `nil`                                      |
| `global.recaptchaSiteKey`                              | Site key for ReCaptcha V2 to verify user's non-robotic behavior. Default value is for testing.                                      | `6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI` |
| `global.recaptchaSiteSecretKey`                        | Secret key for ReCaptcha V2 to verify user's non-robotic behavior. Default value is for testing.                                    | `6LeIxAcTAAAAAGG-vFI1TnRWxMZNFuojJ4WifJWe` |
| `global.bridgeIntegration.enabled`                     | Determines if the bridge integration is enabled. If set to true, the bridge integration will be enabled.                            | `false`                                    |
| `global.bridgeIntegration.baseUrl`                     | The base URL of the bridge api.                                                                                                     | `https://api.bridge.xyz`                   |
| `global.bridgeIntegration.apiKey`                      | The API key for the bridge integration.                                                                                             | `nil`                                      |

### Stellar Disbursement Platform (SDP) parameters

Configuration parameters for the SDP Core Service which is the core backend service that performs several functions:
- Dashboard API: the API used by the front-end UI for all disbursement requests.
- Messaging Service: a recurring process that sends text messages to users prompting them to download the wallet selected for a particular disbursement and verify their phone with an OTP
- Wallet Registration UI: a web application that collects and verifies the recipient's OTP code and verification information via Stellar's SEP-24: Hosted Deposit and Withdrawal protocol

| Name                                                                    | Description                                                                                                                                                    | Value                                           |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `sdp.route`                                                             | Configuration related to the routing of the SDP service.                                                                                                       |                                                 |
| `sdp.route.schema`                                                      | Protocol scheme used for the service. Can be "http" or "https".                                                                                                | `https`                                         |
| `sdp.route.domain`                                                      | Public domain/address of the SDP service. If using localhost, consider including the port as part of the domain.                                               | `nil`                                           |
| `sdp.route.mtnDomain`                                                   | Public domain/address of the multi-tenant SDP service. This is a wild-card domain used for multi-tenant setups e.g. "*.sdp.localhost.com".                     | `nil`                                           |
| `sdp.route.adminDomain`                                                 | Public domain/address of the SDP admin service. Disabled by default. When provided, the admin service will be available at this domain.                        | `nil`                                           |
| `sdp.route.port`                                                        | Primary port on which the SDP service listens.                                                                                                                 | `8000`                                          |
| `sdp.route.metricsPort`                                                 | Port dedicated to metrics collection for the SDP service.                                                                                                      | `8002`                                          |
| `sdp.route.adminPort`                                                   | Port dedicated to serve the SDP admin endpoints, used to manage new or existing tenants.                                                                       | `8003`                                          |
| `sdp.image`                                                             | Configuration related to the Docker image used by the SDP service.                                                                                             |                                                 |
| `sdp.image.repository`                                                  | Docker image repository for the SDP backend service.                                                                                                           | `stellar/stellar-disbursement-platform-backend` |
| `sdp.image.pullPolicy`                                                  | Image pull policy for the SDP service. For locally built images, consider using "Never" or "IfNotPresent".                                                     | `Always`                                        |
| `sdp.image.tag`                                                         | Docker image tag for the SDP service. If set, this overrides the default value from `.Chart.AppVersion`.                                                       | `4.1.0`                                         |
| `sdp.deployment`                                                        | Configuration related to the deployment of the SDP service.                                                                                                    |                                                 |
| `sdp.deployment.annotations`                                            | Annotations to be added to the deployment.                                                                                                                     | `nil`                                           |
| `sdp.deployment.podAnnotations`                                         | Annotations specific to the pods.                                                                                                                              | `{}`                                            |
| `sdp.deployment.podSecurityContext`                                     | Security settings for the pods.                                                                                                                                | `{}`                                            |
| `sdp.deployment.securityContext`                                        | Security settings for the container within the pod.                                                                                                            | `{}`                                            |
| `sdp.deployment.strategy`                                               | Configuration related to the deployment strategy, ensuring smooth updates and minimal downtime.                                                                | `{}`                                            |
| `sdp.deployment.nodeSelector`                                           | Node selector to determine which nodes should run the pods.                                                                                                    | `{}`                                            |
| `sdp.deployment.tolerations`                                            | Tolerations to ensure pods aren't scheduled on unsuitable nodes.                                                                                               | `[]`                                            |
| `sdp.deployment.affinity`                                               | Affinity rules to determine where pods get scheduled based on node conditions.                                                                                 | `{}`                                            |
| `sdp.deployment.priorityClassName`                                      | Name of the priority class to be used by the SDP deployment. If not specified, no priority class will be used.                                                 | `""`                                            |
| `sdp.deployment.topologySpreadConstraints`                              | Pod topology spread constraints for the SDP service, overrides global setting if defined.                                                                      | `[]`                                            |
| `sdp.configMap`                                                         | Configuration for the ConfigMap used by the SDP service.                                                                                                       |                                                 |
| `sdp.configMap.annotations`                                             | Annotations to be added to the ConfigMap.                                                                                                                      | `nil`                                           |
| `sdp.configMap.data`                                                    | Used to inject non-sensitive environment variables into the SDP deployment; for the latest variables, consult the application's CLI `-h` command.              |                                                 |
| `sdp.configMap.data.DISTRIBUTION_PUBLIC_KEY`                            | The public key of the HOST's Stellar distribution account, used to create channel accounts. Required if global.distributionPublicKey not set.                  |                                                 |
| `sdp.configMap.data.SEP10_SIGNING_PUBLIC_KEY`                           | Anchor platform SEP10 signing public key. Required if global.sep10PublicKey not set.                                                                           |                                                 |
| `sdp.configMap.data.RECAPTCHA_SITE_KEY`                                 | Site key for ReCaptcha. Required if using ReCaptcha.                                                                                                           |                                                 |
| `sdp.configMap.data.INSTANCE_NAME`                                      | The name of the SDP instance. Example: "SDP Testnet".                                                                                                          | `SDP Testnet`                                   |
| `sdp.configMap.data.CRASH_TRACKER_TYPE`                                 | Determines the type of crash tracker in use. Options: "DRY_RUN", "SENTRY".                                                                                     | `DRY_RUN`                                       |
| `sdp.configMap.data.ENVIRONMENT`                                        | Specifies the environment SDP is running in (e.g. "localhost").                                                                                                | `dev`                                           |
| `sdp.configMap.data.LOG_LEVEL`                                          | Determines the verbosity level of logs. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "PANIC"                                                   | `INFO`                                          |
| `sdp.configMap.data.METRICS_TYPE`                                       | Defines the type of metrics system in use. Options: "PROMETHEUS".                                                                                              | `PROMETHEUS`                                    |
| `sdp.configMap.data.EMAIL_SENDER_TYPE`                                  | The messenger type used to send invitations to new dashboard users. Options: "DRY_RUN", "AWS_EMAIL", "TWILIO_EMAIL".                                           | `DRY_RUN`                                       |
| `sdp.configMap.data.SMS_SENDER_TYPE`                                    | The messenger type used to send text messages to recipients. Options: "DRY_RUN", "TWILIO_SMS", "TWILIO_WHATSAPP", "AWS_SMS".                                   | `DRY_RUN`                                       |
| `sdp.configMap.data.CORS_ALLOWED_ORIGINS`                               | Specifies the domains allowed to make cross-origin requests. "*" means all domains are allowed.                                                                | `*`                                             |
| `sdp.configMap.data.DISABLE_RECAPTCHA`                                  | Determines if ReCaptcha should be disabled for login ("true" or "false").                                                                                      | `false`                                         |
| `sdp.configMap.data.DISABLE_MFA`                                        | Determines if email-based MFA should be disabled during login ("true" or "false").                                                                             | `false`                                         |
| `sdp.configMap.data.SCHEDULER_PAYMENT_JOB_SECONDS`                      | The interval in seconds for the payment job that syncs payments between the SDP and the TSS.                                                                   | `10`                                            |
| `sdp.configMap.data.SCHEDULER_RECEIVER_INVITATION_JOB_SECONDS`          | The interval in seconds for the receiver invitation job that sends invitations to new receivers. 0 or negative values disable the job.                         | `10`                                            |
| `sdp.configMap.data.MAX_INVITATION_RESEND_ATTEMPTS`                     | The maximum number of times an invitation can be resent. 0 or negative values disable the job.                                                                 | `3`                                             |
| `sdp.configMap.data.TENANT_XLM_BOOTSTRAP_AMOUNT`                        | The amount of XLM to be sent to a newly created tenant distribution account.                                                                                   | `5`                                             |
| `sdp.configMap.data.CIRCLE_API_TYPE`                                    | The type of Circle API to be used. Options: "TRANSFERS", "PAYOUTS". Default: "TRANSFERS".                                                                      | `TRANSFERS`                                     |
| `sdp.configMap.data.ENABLE_BRIDGE_INTEGRATION`                          | Determines if the bridge integration is enabled. If set to true, the bridge integration will be enabled.                                                       |                                                 |
| `sdp.configMap.data.BRIDGE_BASE_URL`                                    | The base URL of the bridge API. Required if ENABLE_BRIDGE_INTEGRATION is set to true.                                                                          |                                                 |
| `sdp.kubeSecrets`                                                       | Kubernetes secrets are used to manage sensitive information, such as API keys and private keys. It's crucial that these details are kept private.              |                                                 |
| `sdp.kubeSecrets.secretName`                                            | The name of the Kubernetes secret object. Only use this if create is false.                                                                                    | `sdp-backend-secret-name`                       |
| `sdp.kubeSecrets.create`                                                | If true, the secret will be created. If false, it is assumed the secret already exists.                                                                        | `false`                                         |
| `sdp.kubeSecrets.annotations`                                           | Annotations to be added to the secret.                                                                                                                         | `nil`                                           |
| `sdp.kubeSecrets.data`                                                  | The sensitive data to be stored in the secret.                                                                                                                 | `{}`                                            |
| `sdp.kubeSecrets.data.DATABASE_URL`                                     | URL of the database used by the SDP.                                                                                                                           |                                                 |
| `sdp.kubeSecrets.data.AWS_ACCESS_KEY_ID`                                | AWS IAM user's access key ID for authenticating to AWS services.                                                                                               |                                                 |
| `sdp.kubeSecrets.data.AWS_REGION`                                       | AWS region where services (like SES for email sending) are provisioned.                                                                                        |                                                 |
| `sdp.kubeSecrets.data.AWS_SECRET_ACCESS_KEY`                            | AWS IAM user's secret access key for authenticating to AWS services.                                                                                           |                                                 |
| `sdp.kubeSecrets.data.AWS_SES_SENDER_ID`                                | Identifier for the AWS SES service used for sending emails.                                                                                                    |                                                 |
| `sdp.kubeSecrets.data.AWS_SNS_SENDER_ID`                                | Identifier for the AWS SNS service used for sending text messages.                                                                                             |                                                 |
| `sdp.kubeSecrets.data.TWILIO_ACCOUNT_SID`                               | Account SID for authenticating to the Twilio service, used for sending text messages.                                                                          |                                                 |
| `sdp.kubeSecrets.data.TWILIO_AUTH_TOKEN`                                | Authentication token for the Twilio service.                                                                                                                   |                                                 |
| `sdp.kubeSecrets.data.TWILIO_SERVICE_SID`                               | Service SID for the specific Twilio service being utilized.                                                                                                    |                                                 |
| `sdp.kubeSecrets.data.TWILIO_WHATSAPP_FROM_NUMBER`                      | The WhatsApp Business number used to send messages (with whatsapp: prefix).                                                                                    |                                                 |
| `sdp.kubeSecrets.data.TWILIO_WHATSAPP_RECEIVER_INVITATION_TEMPLATE_SID` | The Twilio Content SID for WhatsApp receiver invitation template (starts with HX).                                                                             |                                                 |
| `sdp.kubeSecrets.data.TWILIO_WHATSAPP_RECEIVER_OTP_TEMPLATE_SID`        | The Twilio Content SID for WhatsApp receiver OTP template (starts with HX).                                                                                    |                                                 |
| `sdp.kubeSecrets.data.TWILIO_SENDGRID_API_KEY`                          | API key for the Twilio SendGrid (email) service.                                                                                                               |                                                 |
| `sdp.kubeSecrets.data.TWILIO_SENDGRID_SENDER_ADDRESS`                   | Email address used to send emails via Twilio SendGrid.                                                                                                         |                                                 |
| `sdp.kubeSecrets.data.SENTRY_DSN`                                       | The DSN for the Sentry service. it must be set if CRASH_TRACKER_TYPE is set to "SENTRY".                                                                       |                                                 |
| `sdp.kubeSecrets.data.EC256_PRIVATE_KEY`                                | The EC256 Private Key. This key is used to sign the authentication token. This EC key needs to be at least as strong as prime256v1 (P-256).                    |                                                 |
| `sdp.kubeSecrets.data.ANCHOR_PLATFORM_OUTGOING_JWT_SECRET`              | The JWT secret used to create a JWT token used to send requests to the anchor platform.                                                                        |                                                 |
| `sdp.kubeSecrets.data.SEP24_JWT_SECRET`                                 | The JWT secret that's used by the Anchor Platform to sign the SEP-24 JWT token. Must be the same as Anchor Platform's SECRET_SEP24_INTERACTIVE_URL_JWT_SECRET. |                                                 |
| `sdp.kubeSecrets.data.RECAPTCHA_SITE_SECRET_KEY`                        | Secret key for Google reCAPTCHA service to verify user's non-robotic behavior.                                                                                 |                                                 |
| `sdp.kubeSecrets.data.SEP10_SIGNING_PRIVATE_KEY`                        | The public key of the Stellar account that signs the SEP-10 transactions. It's also used to sign URLs. Required if global.sep10PrivateKey not set.             |                                                 |
| `sdp.kubeSecrets.data.DISTRIBUTION_SEED`                                | The HOST's Stellar distribution account, used to create channel accounts. This is needed for the init container.                                               |                                                 |
| `sdp.kubeSecrets.data.DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE`       | A Stellar-compliant ed25519 private key used to encrypt and decrypt the private keys of tenants' distribution accounts.                                        |                                                 |
| `sdp.kubeSecrets.data.CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE`            | The private key used to encrypt the channel accounts secrets in the database.                                                                                  |                                                 |
| `sdp.kubeSecrets.data.KAFKA_SASL_USERNAME`                              | The username for SASL authentication to the Kafka broker. Required if KAFKA_SECURITY_PROTOCOL is set to "SASL_SSL" or "SASL_PLAINTEXT".                        |                                                 |
| `sdp.kubeSecrets.data.KAFKA_SASL_PASSWORD`                              | The password for SASL authentication to the Kafka broker. Required if KAFKA_SECURITY_PROTOCOL is set to "SASL_SSL" or "SASL_PLAINTEXT".                        |                                                 |
| `sdp.kubeSecrets.data.KAFKA_SSL_ACCESS_KEY`                             | Access key (keystore) in PEM format. Required if KAFKA_SECURITY_PROTOCOL is set to "SSL".                                                                      |                                                 |
| `sdp.kubeSecrets.data.KAFKA_SSL_ACCESS_CERTIFICATE`                     | Certificate in PEM format that matches with the Kafka Access Key. Required if KAFKA_SECURITY_PROTOCOL is set to "SSL".                                         |                                                 |
| `sdp.kubeSecrets.data.ADMIN_ACCOUNT`                                    | The ID of the admin account. To use, add to the request header as 'Authorization', formatted as Base64-encoded 'ADMIN_ACCOUNT:ADMIN_API_KEY'.",                |                                                 |
| `sdp.kubeSecrets.data.ADMIN_API_KEY`                                    | The API key for the admin account. To use, add to the request header as 'Authorization', formatted as Base64-encoded 'ADMIN_ACCOUNT:ADMIN_API_KEY'.",          |                                                 |
| `sdp.kubeSecrets.data.BRIDGE_API_KEY`                                   | The API key for the bridge integration. Required if ENABLE_BRIDGE_INTEGRATION is set to true.                                                                  |                                                 |
| `sdp.ingress`                                                           | Configuration for the ingress controller for the SDP service.                                                                                                  |                                                 |
| `sdp.ingress.enabled`                                                   | If true, an ingress controller will be created for the SDP service.                                                                                            | `true`                                          |
| `sdp.ingress.className`                                                 | Name of the IngressClass to be used for the ingress controller.                                                                                                | `nginx`                                         |
| `sdp.ingress.tls[0].hosts`                                              | List of hosts covered by the TLS certificate.                                                                                                                  | `["{{ include \"sdp.domain\" . }}"]`            |
| `sdp.ingress.tls[0].secretName`                                         | The name of the Kubernetes TLS secret. You need to create this secret manually.                                                                                | `backend-tls-cert-name`                         |

### Anchor Platform

Configuration parameters for the Anchor Platform which is the API server that the wallet uses to authenticate and initiate
the recipient's registration process through the SEP-24 deposit flow.

| Name                                                                      | Description                                                                                                                                                                                                                                                                                                              | Value                                   |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------- |
| `anchorPlatform.route`                                                    | Configuration related to the routing of the Anchor Platform service.                                                                                                                                                                                                                                                     |                                         |
| `anchorPlatform.route.schema`                                             | Protocol scheme used for the service. Can be "http" or "https".                                                                                                                                                                                                                                                          | `https`                                 |
| `anchorPlatform.route.domain`                                             | Public domain/address of the Anchor Platform service. If using localhost, consider including the port as part of the domain.                                                                                                                                                                                             | `nil`                                   |
| `anchorPlatform.route.sepPort`                                            | The port of the sep server of the anchor platform. This is the public API that is meant to be reached by a client application, such as the stellar.toml file."                                                                                                                                                           | `8080`                                  |
| `anchorPlatform.route.platformPort`                                       | The port of the platform server of the anchor platform. This is the private API that is meant to be reached only by the SDP server, such as the PATCH /sep24/transactions endpoint.",                                                                                                                                    | `8085`                                  |
| `anchorPlatform.image`                                                    | Configuration related to the Docker image used by the Anchor Platform service.                                                                                                                                                                                                                                           |                                         |
| `anchorPlatform.image.repository`                                         | Docker image repository for the Anchor Platform service.                                                                                                                                                                                                                                                                 | `stellar/anchor-platform`               |
| `anchorPlatform.image.pullPolicy`                                         | Image pull policy for the Anchor Platform service.                                                                                                                                                                                                                                                                       | `IfNotPresent`                          |
| `anchorPlatform.image.tag`                                                | Docker image tag for the Anchor Platform service.                                                                                                                                                                                                                                                                        | `2.6.2`                                 |
| `anchorPlatform.deployment`                                               | Configuration related to the deployment of the Anchor Platform.                                                                                                                                                                                                                                                          |                                         |
| `anchorPlatform.deployment.annotations`                                   | Annotations to be added to the deployment.                                                                                                                                                                                                                                                                               | `{}`                                    |
| `anchorPlatform.deployment.podAnnotations`                                | Annotations specific to the pods.                                                                                                                                                                                                                                                                                        | `{}`                                    |
| `anchorPlatform.deployment.strategy`                                      | Configuration related to the deployment strategy, ensuring smooth updates and minimal downtime.                                                                                                                                                                                                                          | `{}`                                    |
| `anchorPlatform.deployment.podSecurityContext`                            | Security settings for the pods.                                                                                                                                                                                                                                                                                          | `{}`                                    |
| `anchorPlatform.deployment.securityContext`                               | Security settings for the container within the pod.                                                                                                                                                                                                                                                                      | `{}`                                    |
| `anchorPlatform.deployment.resources`                                     | Resource limits and requests for the application pods.                                                                                                                                                                                                                                                                   | `{}`                                    |
| `anchorPlatform.deployment.nodeSelector`                                  | Node selector to determine which nodes should run the pods.                                                                                                                                                                                                                                                              | `{}`                                    |
| `anchorPlatform.deployment.tolerations`                                   | Tolerations to ensure pods aren't scheduled on unsuitable nodes.                                                                                                                                                                                                                                                         | `[]`                                    |
| `anchorPlatform.deployment.affinity`                                      | Affinity rules to determine where pods get scheduled based on node conditions.                                                                                                                                                                                                                                           | `{}`                                    |
| `anchorPlatform.deployment.priorityClassName`                             | Name of the priority class to be used by the Anchor Platform deployment. If not specified, no priority class will be used.                                                                                                                                                                                               | `""`                                    |
| `anchorPlatform.deployment.topologySpreadConstraints`                     | Pod topology spread constraints for the Anchor Platform service, overrides global setting if defined.                                                                                                                                                                                                                    | `[]`                                    |
| `anchorPlatform.configMap`                                                | Configuration for the ConfigMap used by the anchorPlatform service.                                                                                                                                                                                                                                                      |                                         |
| `anchorPlatform.configMap.annotations`                                    | Annotations to be added to the ConfigMap.                                                                                                                                                                                                                                                                                | `nil`                                   |
| `anchorPlatform.configMap.data`                                           | Used to inject non-sensitive environment variables into the Anchor Platform deployment; for the latest variables, consult Anchor Platform's public documentation.                                                                                                                                                        |                                         |
| `anchorPlatform.configMap.data.APP_LOGGING_LEVEL`                         | Specifies the logging level for the application (e.g. "INFO", "DEBUG", "ERROR").                                                                                                                                                                                                                                         | `INFO`                                  |
| `anchorPlatform.configMap.data.DATA_DATABASE`                             | Specifies the database connection details for the platform. Will be auto-populated in the development helm chart when `ephemeralDatabase` is enabled.                                                                                                                                                                    |                                         |
| `anchorPlatform.configMap.data.DATA_SERVER`                               | Specifies the server connection details for the platform. Will be auto-populated in the development helm chart when `ephemeralDatabase` is enabled.                                                                                                                                                                      |                                         |
| `anchorPlatform.configMap.data.DATA_FLYWAY_ENABLED`                       | Determines if Flyway, the database migration tool, is enabled.                                                                                                                                                                                                                                                           |                                         |
| `anchorPlatform.configMap.data.ASSETS_VALUE`                              | Specifies the details and configuration of assets supported by the anchor platform. This includes SEP-24 enabled assets, schema type, code, issuer details, distribution account, precision details, and deposit and withdrawal configurations. Currently, it needs to be *manually* kept up to date with the SDP state. |                                         |
| `anchorPlatform.configMap.data.DATA_DDL_AUTO`                             | Specifies the strategy Hibernate should use for the database schema initialization. The standard Hibernate property values are `none`, `validate`, `update`, `create-drop`.                                                                                                                                              | `update`                                |
| `anchorPlatform.configMap.data.METRICS_ENABLED`                           | Determines if metrics collection is enabled for the platform. If enabled, metrics would be available at port 8082.                                                                                                                                                                                                       | `false`                                 |
| `anchorPlatform.configMap.data.METRICS_EXTRAS_ENABLED`                    | Determines if additional metrics (beyond the standard set) are enabled for collection.                                                                                                                                                                                                                                   | `false`                                 |
| `anchorPlatform.configMap.data.SEP10_CLIENT_ATTRIBUTION_REQUIRED`         | When set to `true`, only SEP-10 requests from known clients listed in `SEP10_CLIENT_ATTRIBUTION_ALLOW_LIST` will be accepted.                                                                                                                                                                                            | `false`                                 |
| `anchorPlatform.configMap.data.SEP10_CLIENT_ATTRIBUTION_ALLOW_LIST`       | The comma-separated list of client domains allowed to make SEP-10 requests.                                                                                                                                                                                                                                              | `""`                                    |
| `anchorPlatform.kubeSecrets`                                              | secrets are used to manage sensitive information, such as API keys and private keys. It's crucial that these details are kept private.                                                                                                                                                                                   |                                         |
| `anchorPlatform.kubeSecrets.secretName`                                   | The name of the Kubernetes secret object. Only use this if create is false.                                                                                                                                                                                                                                              | `anchor-platform-secret-name`           |
| `anchorPlatform.kubeSecrets.create`                                       | If true, the secret will be created. If false, it is assumed the secret already exists.                                                                                                                                                                                                                                  | `false`                                 |
| `anchorPlatform.kubeSecrets.annotations`                                  | Annotations to be added to the secret.                                                                                                                                                                                                                                                                                   | `nil`                                   |
| `anchorPlatform.kubeSecrets.data`                                         | The sensitive data to be stored in the secret.                                                                                                                                                                                                                                                                           | `{}`                                    |
| `anchorPlatform.kubeSecrets.data.SECRET_DATA_PASSWORD`                    | Database password for the anchor platform.                                                                                                                                                                                                                                                                               |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_DATA_USERNAME`                    | Database username for the anchor platform.                                                                                                                                                                                                                                                                               |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_PLATFORM_API_AUTH_SECRET`         | The secret used for authenticating API requests between the SDP and the Anchor Platform.                                                                                                                                                                                                                                 |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_SEP10_JWT_SECRET`                 | The JWT secret used by the Anchor Platform to sign SEP-10 JWT tokens. These tokens are used for various authentication and transaction-related purposes.                                                                                                                                                                 |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_SEP10_SIGNING_SEED`               | The seed for the SEP-10 signing process. It's essential for ensuring the security and authenticity of SEP-10 transactions. Required if global.sep10PrivateKey not set.                                                                                                                                                   |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_SEP24_INTERACTIVE_URL_JWT_SECRET` | The JWT secret used by the Anchor Platform to sign SEP-24 interactive URLs. These URLs typically initiate user-interactive processes like deposits and withdrawals. Must be the same as SDP's SEP24_JWT_SECRET.                                                                                                          |                                         |
| `anchorPlatform.kubeSecrets.data.SECRET_SEP24_MORE_INFO_URL_JWT_SECRET`   | The JWT secret used by the Anchor Platform to sign SEP-24 'More Info' URLs. These URLs provide users with additional details or steps related to their transactions.                                                                                                                                                     |                                         |
| `anchorPlatform.ingress`                                                  | Configuration for the ingress controller for the Anchor Platform.                                                                                                                                                                                                                                                        |                                         |
| `anchorPlatform.ingress.enabled`                                          | If true, an ingress controller will be created for the Anchor Platform.                                                                                                                                                                                                                                                  | `true`                                  |
| `anchorPlatform.ingress.className`                                        | Name of the IngressClass to be used for the ingress controller.                                                                                                                                                                                                                                                          | `nginx`                                 |
| `anchorPlatform.ingress.tls[0].hosts`                                     | List of hosts covered by the TLS certificate.                                                                                                                                                                                                                                                                            | `["{{ include \"sdp.ap.domain\" . }}"]` |
| `anchorPlatform.ingress.tls[0].secretName`                                | The name of the Kubernetes TLS secret. You need to create this secret manually. For more instructions, please refer to helmchart/docs/README.md                                                                                                                                                                          | `backend-tls-cert-name`                 |

### Transaction Submission Service

Configuration parameters for the Transaction Submission Service. This is the service that submits all payment transactions to the Stellar network.
This service is designed to maximize payment throughput, handle queuing, and graceful resubmission/error handling

| Name                                                              | Description                                                                                                                                                            | Value             |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------- |
| `tss.enabled`                                                     | If true, the tss will be deployed.                                                                                                                                     | `true`            |
| `tss.route`                                                       | Configuration related to the routing of the TSS.                                                                                                                       |                   |
| `tss.route.schema`                                                | Protocol scheme used for the service. Can be "http" or "https".                                                                                                        | `https`           |
| `tss.route.port`                                                  | Primary port on which the TSS listens.                                                                                                                                 | `9000`            |
| `tss.route.metricsPort`                                           | Port dedicated to metrics collection for the TSS.                                                                                                                      | `9002`            |
| `tss.deployment`                                                  | Configuration related to the deployment of the TSS.                                                                                                                    |                   |
| `tss.deployment.annotations`                                      | Annotations to be added to the deployment.                                                                                                                             | `nil`             |
| `tss.deployment.podAnnotations`                                   | Annotations specific to the pods.                                                                                                                                      | `{}`              |
| `tss.deployment.strategy`                                         | Configuration related to the deployment strategy, ensuring smooth updates and minimal downtime.                                                                        | `{}`              |
| `tss.deployment.podSecurityContext`                               | Security settings for the pods.                                                                                                                                        | `{}`              |
| `tss.deployment.securityContext`                                  | Security settings for the container within the pod.                                                                                                                    | `{}`              |
| `tss.deployment.resources`                                        | Resource limits and requests for the application pods.                                                                                                                 | `{}`              |
| `tss.deployment.nodeSelector`                                     | Node selector to determine which nodes should run the pods.                                                                                                            | `{}`              |
| `tss.deployment.tolerations`                                      | Tolerations to ensure pods aren't scheduled on unsuitable nodes.                                                                                                       | `[]`              |
| `tss.deployment.affinity`                                         | Affinity rules to determine where pods get scheduled based on node conditions.                                                                                         | `{}`              |
| `tss.deployment.priorityClassName`                                | Name of the priority class to be used by the TSS deployment. If not specified, no priority class will be used.                                                         | `""`              |
| `tss.deployment.topologySpreadConstraints`                        | Pod topology spread constraints for the TSS service, overrides global setting if defined.                                                                              | `[]`              |
| `tss.configMap`                                                   | Configuration settings for the Transaction Submission Service (TSS) ConfigMap.                                                                                         |                   |
| `tss.configMap.annotations`                                       | Annotations to be added to the ConfigMap.                                                                                                                              | `nil`             |
| `tss.configMap.data`                                              | Used to inject non-sensitive environment variables into the TSS deployment; for the latest variables, consult the application's CLI `-h` command.                      |                   |
| `tss.configMap.data.DISTRIBUTION_PUBLIC_KEY`                      | The public key of the HOST's Stellar distribution account, used to create channel accounts. Required if global.distributionPublicKey not set.                          |                   |
| `tss.configMap.data.CRASH_TRACKER_TYPE`                           | Determines the type of crash tracker in use. Options: "DRY_RUN", "SENTRY".                                                                                             | `DRY_RUN`         |
| `tss.configMap.data.ENVIRONMENT`                                  | Specifies the environment TSS is running in (e.g. "localhost").                                                                                                        | `development`     |
| `tss.configMap.data.LOG_LEVEL`                                    | Determines the verbosity level of logs. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "PANIC"                                                           | `INFO`            |
| `tss.configMap.data.TSS_METRICS_TYPE`                             | Defines the type of metrics system that the TSS should use. Options: "TSS_PROMETHEUS".                                                                                 | `TSS_PROMETHEUS`  |
| `tss.configMap.data.NUM_CHANNEL_ACCOUNTS`                         | The number of channel accounts the TSS will create/use. Channel accounts provide a method for submitting transactions to the network at a high rate.                   | `1`               |
| `tss.configMap.data.MAX_BASE_FEE`                                 | Specifies the maximum base fee (in stroops) the TSS is willing to pay per transaction. This helps to control costs and ensures transactions are economically feasible. | `100000`          |
| `tss.configMap.data.QUEUE_POLLING_INTERVAL`                       | Specifies the interval (in seconds) at which the TSS should poll the queue.                                                                                            | `6`               |
| `tss.kubeSecrets`                                                 | Kubernetes secrets are used to manage sensitive information, such as API keys and private keys. It's crucial that these details are kept private.                      |                   |
| `tss.kubeSecrets.secretName`                                      | The name of the Kubernetes secret object. Only use this if create is false.                                                                                            | `tss-secret-name` |
| `tss.kubeSecrets.create`                                          | If true, the secret will be created. If false, it is assumed the secret already exists.                                                                                | `false`           |
| `tss.kubeSecrets.annotations`                                     | Annotations to be added to the secret.                                                                                                                                 | `{}`              |
| `tss.kubeSecrets.data`                                            | The sensitive data to be stored in the secret.                                                                                                                         | `{}`              |
| `tss.kubeSecrets.data.DATABASE_URL`                               | URL of the database used by the TSS.                                                                                                                                   |                   |
| `tss.kubeSecrets.data.DISTRIBUTION_SEED`                          | The HOST's Stellar distribution account, used to create channel accounts.                                                                                              |                   |
| `tss.kubeSecrets.data.CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE`      | The private key used to encrypt the channel accounts secrets in the database.                                                                                          |                   |
| `tss.kubeSecrets.data.DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE` | A Stellar-compliant ed25519 private key used to encrypt and decrypt the private keys of tenants' distribution accounts.                                                |                   |
| `tss.kubeSecrets.data.SENTRY_DSN`                                 | The DSN for the Sentry service. it must be set if CRASH_TRACKER_TYPE is set to "SENTRY".                                                                               |                   |
| `tss.kubeSecrets.data.KAFKA_SASL_USERNAME`                        | The username for SASL authentication to the Kafka broker. Required if KAFKA_SECURITY_PROTOCOL is set to "SASL_SSL" or "SASL_PLAINTEXT".                                |                   |
| `tss.kubeSecrets.data.KAFKA_SASL_PASSWORD`                        | The password for SASL authentication to the Kafka broker. Required if KAFKA_SECURITY_PROTOCOL is set to "SASL_SSL" or "SASL_PLAINTEXT".                                |                   |
| `tss.kubeSecrets.data.KAFKA_SSL_ACCESS_KEY`                       | Access key (keystore) in PEM format. Required if KAFKA_SECURITY_PROTOCOL is set to "SSL".                                                                              |                   |
| `tss.kubeSecrets.data.KAFKA_SSL_ACCESS_CERTIFICATE`               | Certificate in PEM format that matches with the Kafka Access Key. Required if KAFKA_SECURITY_PROTOCOL is set to "SSL".                                                 |                   |

### Dashboard

Configuration parameters for the Dashboard. This is the user interface administrators use to initiate and track the progress of disbursements.

| Name                                             | Description                                                                                                                                        | Value                                                  |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `dashboard.enabled`                              | If true, the dashboard will be deployed.                                                                                                           | `true`                                                 |
| `dashboard.route`                                | Configuration related to the routing of the Dashboard.                                                                                             |                                                        |
| `dashboard.route.schema`                         | Protocol scheme used for the service. Can be "http" or "https".                                                                                    | `https`                                                |
| `dashboard.route.domain`                         | Public domain/address of the Dashboard.                                                                                                            | `nil`                                                  |
| `dashboard.route.mtnDomain`                      | Public domain/address of the multi-tenant Dashboard. This is a wild-card domain used for multi-tenant setups e.g. "*.sdp-dashboard.localhost.com". | `nil`                                                  |
| `dashboard.route.port`                           | Primary port on which the Dashboard listens.                                                                                                       | `80`                                                   |
| `dashboard.image`                                | Configuration related to the Docker image used by the Dashboard.                                                                                   |                                                        |
| `dashboard.image.fullName`                       | Full name of the Docker image.                                                                                                                     | `stellar/stellar-disbursement-platform-frontend:4.1.0` |
| `dashboard.image.pullPolicy`                     | Image pull policy for the dashboard. For locally built images, consider using "Never" or "IfNotPresent".                                           | `Always`                                               |
| `dashboard.deployment`                           | Configuration related to the deployment of the Dashboard.                                                                                          |                                                        |
| `dashboard.deployment.annotations`               | Annotations to be added to the deployment.                                                                                                         | `{}`                                                   |
| `dashboard.deployment.podAnnotations`            | Annotations specific to the pods.                                                                                                                  | `{}`                                                   |
| `dashboard.deployment.strategy`                  | Configuration related to the deployment strategy, ensuring smooth updates and minimal downtime.                                                    | `{}`                                                   |
| `dashboard.deployment.podSecurityContext`        | Security settings for the pods.                                                                                                                    | `{}`                                                   |
| `dashboard.deployment.securityContext`           | Security settings for the container within the pod.                                                                                                | `{}`                                                   |
| `dashboard.deployment.resources`                 | Resource limits and requests for the application pods.                                                                                             | `{}`                                                   |
| `dashboard.deployment.nodeSelector`              | Node selector to determine which nodes should run the pods.                                                                                        | `{}`                                                   |
| `dashboard.deployment.tolerations`               | Tolerations to ensure pods aren't scheduled on unsuitable nodes.                                                                                   | `[]`                                                   |
| `dashboard.deployment.affinity`                  | Affinity rules to determine where pods get scheduled based on node conditions.                                                                     | `{}`                                                   |
| `dashboard.deployment.priorityClassName`         | Name of the priority class to be used by the Dashboard deployment. If not specified, no priority class will be used.                               | `""`                                                   |
| `dashboard.deployment.topologySpreadConstraints` | Pod topology spread constraints for the Dashboard service, overrides global setting if defined.                                                    | `[]`                                                   |
| `dashboard.configMap`                            | Configuration settings for the Dashboard ConfigMap.                                                                                                |                                                        |
| `dashboard.configMap.annotations`                | Annotations to be added to the ConfigMap.                                                                                                          | `{}`                                                   |
| `dashboard.configMap.data`                       | Used to inject non-sensitive environment variables into the Dashboard deployment.                                                                  | `{}`                                                   |
| `dashboard.configMap.data.RECAPTCHA_SITE_KEY`    | The site key for Google reCAPTCHA service.                                                                                                         |                                                        |
| `dashboard.ingress`                              | Configuration for the ingress controller for the dashboard.                                                                                        |                                                        |
| `dashboard.ingress.enabled`                      | If true, an ingress controller will be created for the dashboard.                                                                                  | `true`                                                 |
| `dashboard.ingress.className`                    | Name of the IngressClass to be used for the ingress controller.                                                                                    | `nginx`                                                |
| `dashboard.ingress.tls[0].hosts`                 | List of hosts covered by the TLS certificate.                                                                                                      | `["{{ include \"sdp.dashboard.domain\" . }}"]`         |
| `dashboard.ingress.tls[0].secretName`            | The name of the Kubernetes TLS secret. You need to create this secret manually.                                                                    | `dashboard-tls-cert-name`                              |
