# Stellar Disbursement Platform on AWS ECS
Reece Markowksy: Stellar Engineering Manager
The Stellar Disbursement Platform (SDP) is a powerful tool for organizations that must distribute digital assets to recipients at scale. In this post, I'll describe how to deploy on AWS using Cloud Formation stacks.  The goal is to provide you with a streamlined ECS deployment to help get you started.
# Cloud Formation Stack Overview
Our deployment uses a modular CloudFormation stack design that builds the platform in layers:
* one-click - the stack you deploy that will deploy all other (nested) stacks:
  * Network Stack - infrastructure (VPC, subnets, security groups)
  * Database tier (PostgreSQL RDS)
  * Key management Stack  (Stellar account keys and encryption secrets with auto-generation lambda function)
  * ECS Stack - ECS containers for frontend, backend, TSS, and anchor platform)
* Tenant creator stack - Lambda-based setup of the default organization using the private network /tenants API.

# Step-by-Step Deployment:
Let me walk you through deploying the Stellar Disbursement Platform (SDP) using our CloudFormation stacks.
# Prerequisites
Before starting the deployment, you need:
* An AWS account with administrative permissions 
* A domain registered in Route53 with a hosted zone 
* An ACM certificate for your domain (must be in the same region as your deployment)
* An EC2 key pair for SSH access to the bastion host

⠀Deployment Steps
### 1. Prepare the deployment parameters
First, create a parameters file similar to the sdp-one-click-parameters.json included in your stack set:

```json
[
  {
    "ParameterKey": "Environment",
    "ParameterValue": "dev"
  },
  {
    "ParameterKey": "DomainName",
    "ParameterValue": "sdp.example.org"
  },
  {
    "ParameterKey": "HostedZoneId",
    "ParameterValue": "Z0123456789ABCDEFGHIJ"
  },
  {
    "ParameterKey": "CertificateArn",
    "ParameterValue": "arn:aws:acm:us-west-2:123456789012:certificate/abcd1234-5678-90ab-cdef-1234567890ab"
  },
  {
    "ParameterKey": "DBInstanceClass",
    "ParameterValue": "db.t3.small"
  },
  {
    "ParameterKey": "DBUsername",
    "ParameterValue": "sdpadmin"
  },
  {
    "ParameterKey": "DBPassword",
    "ParameterValue": "StrongPassword123!"
  },
  {
    "ParameterKey": "BackupRetentionPeriod",
    "ParameterValue": "7"
  },
  {
    "ParameterKey": "MultiAZ",
    "ParameterValue": "false"
  },
  {
    "ParameterKey": "AdminApiKey",
    "ParameterValue": "api-key-for-admin-access-123"
  },
  {
    "ParameterKey": "StellarLayerS3Bucket",
    "ParameterValue": "my-stellar-layer-bucket"
  },
  {
    "ParameterKey": "TenantName",
    "ParameterValue": "acme"
  },
  {
    "ParameterKey": "TenantOwnerEmail",
    "ParameterValue": "admin@example.org"
  },
  {
    "ParameterKey": "TenantOwnerFirstName",
    "ParameterValue": "Admin"
  },
  {
    "ParameterKey": "TenantOwnerLastName",
    "ParameterValue": "User"
  },
  {
    "ParameterKey": "TenantOrganizationName",
    "ParameterValue": "ACME Foundation"
  },
  {
    "ParameterKey": "KeyName",
    "ParameterValue": "my-ec2-key"
  }
]
```

### 2. Create the CloudFormation stack
Use the AWS CLI or AWS Management Console to create the stack:
### bash

aws cloudformation create-stack \
  --stack-name stellar-disbursement-platform\
  --parameters file://sdp-one-click-parameters.json \
  --capabilities CAPABILITY_IAM

### 3. Monitor the deployment
The deployment will take approximately 30-45 minutes to complete. You can monitor progress:

```bash
aws cloudformation describe-stacks --stack-name sdp-deployment --query "Stacks[0].StackStatus"
```
Or watch the stack events in the AWS Management Console.
### 5. Access the deployed platform
Once deployment is complete, you'll see the output URLs in the CloudFormation stack outputs. The main access points are:
* Admin UI: https://sdp-frontend.sdp.example.org
* API: https://sdp-backend.sdp.example.org
* Anchor Platform: https://anchor.sdp.example.org

# Setting Up the Default Tenant
The stack automatically creates your default tenant based on the parameters you provided. After deployment, you'll need to complete the account setup:
### 1. Check your email
The email will:
* Welcome you to the Stellar Disbursement Platform
* Inform you that you've been added as an owner
* Provide a button to set up your password
![](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/Screenshot%202025-04-08%20at%209.17.45%E2%80%AFPM.jpeg)<!-- {"width":370} -->
### 2. Set up your password
1 Click the "Set up my password" button in the email
2 You'll be directed to a password creation page
3 After setting your password, you'll be redirected to the login page

![](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/image%202.png)<!-- {"width":370} -->
⠀![](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/image%203.png)

Go back to the login screen and log-in with your new password. You will also need to submit the Verification OTP Code sent to your email.

![](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/image%205.png)<!-- {"width":430} -->

Once logged in you will see the dashboard. Go ahead and send your first disbursement!
![](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/image%204.png)
The following video demonstrates using demo wallet to send a test disbursement
[Screen Recording 2025-04-09 at 8.12.29 AM.mov](Stellar%20Disbursement%20Platform%20on%20AWS%20ECS/Screen%20Recording%202025-04-09%20at%208.12.29%E2%80%AFAM.mov)<!-- {"embed":"true"} -->

Troubleshooting Tips
If you encounter issues during deployment:
**1** **Stack creation fails**: Check the CloudFormation events for specific error messages. Common issues include:
	* Missing permissions for creating IAM roles
	* S3 bucket or templates are not accessible
	* Certificate ARN is incorrect or in a different region
**2** **Services not accessible after deployment**: Check:
	* DNS propagation (can take up to 48 hours, though typically much faster)
	* Security group rules to ensure traffic is permitted
	* Log into the bastion host to check connectivity to services internally
**3** **Default tenant not created**: The tenant creator Lambda might have failed. Check:
	* Lambda logs in CloudWatch
	* Connectivity from the Lambda to the SDP backend API
	* API key configuration

## Next Steps After Deployment
Once your SDP instance is up and running:
1 Set up your Stellar distribution account with funds
2 Configure your disbursement programs
3 Test the end-to-end flow with a small sample group
4 Set up monitoring and alerting for production use

# Design Considerations
### Network Design
The architecture follows AWS best practices with a multi-AZ deployment across private and public subnets:
* **Public subnets** host only the Application Load Balancers
* **Private subnets** contain all application components and the database
* **Security groups** enforce traffic isolation between components
* **NAT Gateway** provides outbound access from private subnets
The public-facing ALB terminates HTTPS traffic and routes requests to appropriate services based on hostname and path patterns.
### Database Tier
The PostgreSQL RDS instance provides persistent storage for the platform:
* Configurable instance size (t3.small to r5.large)
* Optional Multi-AZ deployment for high availability
* Backup retention policies
* Encryption at rest
### Security & Key Management
The SDP requires several Stellar keypairs and encryption secrets, which are stored in AWS Secrets Manager:
* Distribution account keypair (for sending assets on Stellar)
* SEP-10 signing keypair (for authentication)
* JWT secrets for various authentication flows
* Encryption passphrases for securing sensitive data
⠀The architecture provides options to either auto-generate secure keys or use pre-existing keys.
### Application Services
The platform runs as containerized services on ECS Fargate:
* **Frontend**: The web UI for platform administrators
* **Backend**: Core SDP API services and wallet registration
* **TSS (Transaction Submission Service)**: Manages Stellar transactions
* **Anchor Platform**: Implements Stellar SEP protocols for wallet connectivity
### DNS Configuration
Route53 records are created for all service endpoints:
* sdp-frontend.domain.com: Admin interface
* sdp-backend.domain.com: API services
* anchor.domain.com: SEP protocol endpoints for wallet connectivity
* Wildcard record for tenant subdomains
### Security
The architecture emphasizes security through multiple layers:
* All services run in private subnets
* Network traffic is tightly controlled via security groups
* Secrets are managed in AWS Secrets Manager
* Certificates secure all communications
### Operations
The architecture includes features to support operations:
* CloudWatch Logs for centralized logging
* A bastion host for secure database access
* Backup and retention policies

Areas for Improvement
While this architecture provides a solid foundation, there are opportunities for enhancement:
**1** **Observability**: Adding a dedicated monitoring stack with Prometheus/Grafana would provide better visibility into system health.
**2** **CI/CD Pipeline**: Integrating with a CI/CD system would streamline ongoing updates.
**3** **Disaster Recovery**: Implementing cross-region replication would improve resilience against regional failures.
**4** **Cost Optimization**: The default t3.small database might not be sufficient for production loads, but is cost-effective for development. A proper sizing exercise should be conducted for production.
**5** **Auto-Scaling**: The current setup has fixed capacity; implementing auto-scaling based on load metrics would improve resource utilization.
**6** **Enhanced Security**: Implementing AWS WAF and Shield for the public ALB would provide additional protection against common web exploits and DDoS attacks.

⠀Conclusion
This AWS CloudFormation-based deployment of the Stellar Disbursement Platform provides a solid foundation for organizations looking to leverage Stellar for asset distribution. The architecture balances security, scalability, and operational considerations while providing flexibility for different deployment scenarios.
For organizations just starting with SDP, this template set dramatically reduces the time to deploy a production-ready environment, allowing teams to focus on their core mission rather than infrastructure concerns.
