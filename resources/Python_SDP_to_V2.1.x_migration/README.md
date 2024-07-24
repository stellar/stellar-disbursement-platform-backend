# Migration Guide: SDP Aid (Python) to V2.1.x
## Preparation

1. **Backup Data**:
   - Backup your existing SDP Aid database to ensure data safety.

2. **Wildcard TLS Certificates**:
   Multi-tenancy requires wildcard TLS certificates to facilitate tenant provisioning as the SDP relies on subdomains to differentiate between tenants. This will allow you to provision tenants without having to manually configure TLS certificates for each tenant. You can use a service like Let's Encrypt or Namecheap to acquire these certificates.

   For example, if your base domain for running the SDP backend is sdp-prod.domain.cloud, you can provision one tenant per organization. This results in subdomains such as:
    - tenantA.sdp-prod.domain.cloud for the tenant A
    - tenantB.sdp-prod.domain.cloud for the tenant B

    Similarly, acquiring wildcard certificates for the dashboard service (front-end) would simplify accessing each tenant’s dashboard via their fully qualified name. However, this is not strictly necessary, as the dashboards include a field to specify the tenant when accessed through the base domain.


## MIGRATION PART 1/2: Migrating SDP Aid (Python) to SDP Single-tenant (v1.x)


### Data Migration 

1. **Clone and Set Up**:
   - Clone the backend repository:
     ```bash
     git clone https://github.com/stellar/stellar-disbursement-platform-backend
     ```
   - Set the database URL:
     ```bash
     export DATABASE_URL={path-to-postgres-DB-URL-to-migrate}
     ```
   - Run database migrations:
     ```bash
     go run main.go db migrate up
     go run main.go db auth migrate up
     ```

2. **Assets Table**:
   - Update the pubnet USDC asset:
     ```sql
     UPDATE assets SET issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN' WHERE code = 'USDC' AND issuer = 'GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5';
     ```

3. **Wallets Table**:
   - Update the wallets table for the Vibrant Assist wallet:
     ```sql
     UPDATE wallets SET homepage = 'https://vibrantapp.com/vibrant-assist', deep_link_schema = 'https://vibrantapp.com/sdp', sep_10_client_domain = 'api.vibrantapp.com' WHERE name = 'Vibrant Assist';
     ```

4. **Organizations Table**:
   - Update organization details:
     ```sql
     UPDATE organizations SET name = '<your-org-name>', sms_registration_message_template = 'You have a payment waiting for your from <your-org-name>, register at', otp_message_template = 'is your registration code.';
     ```

5. **Date of Birth**:
   - Update date of birth field for receivers:
     ```sql
     UPDATE receiver_verifications SET hashed_value = crypt('2000-01-22', gen_salt('bf', 4)) WHERE receiver_id = (SELECT id FROM receivers WHERE phone_number = '+14155555555' LIMIT 1);
     ```

## Step 2: Migrating SDP V1.x to V2.1.x

### Preparation

2. **Helm Values**:
   - Update the `values.yaml` file with the following values:
     ```yaml
     global.eventBroker.type: "NONE"
     sdp.route.mtnDomain: "*.sdp-prod.uni-cc.cloud"
     sdp.configMap.data.INSTANCE_NAME: "<Insert a name>"
     sdp.configMap.data.ENABLE_SCHEDULER: true
     tss.enabled: true
     dashboard.route.mtnDomain: "*.sdp-prod-dashboard.uni-cc.cloud"
     ```

### Upgrading SDP

1. **Verify Helm Chart Version**:
   - Check the current version:
     ```bash
     helm list --namespace <sdp-namespace>
     ```

2. **Pull Latest Helm Chart**:
   - Update the helm repository and pull the latest version:
     ```bash
     helm repo update
     helm pull stellar/stellar-disbursement-platform
     helm search repo stellar/stellar-disbursement-platform --versions
     ```

3. **Run the Upgrade**:
   - Delete the TSS deployment and upgrade:
     ```bash
     kubectl delete deployment <your-app-name>-tss -n <your-namespace>
     helm upgrade --install <your-app-name> --namespace <your-name-space> --debug -f <your-values-file>.yaml stellar/stellar-disbursement-platform --version 2.0.1
     ```

### New Tenant Provisioning

1. **Create New Tenant**:
   - Use the Admin API to create a new tenant:
     ```bash
     curl -X POST <your-sdp-backend-url>:8003/tenants \
     -H "Content-Type: application/json" \
     -H "Authorization: Basic ${Base64(ADMIN_ACCOUNT:ADMIN_API_KEY)}" \
     -d '{
       "name": "<your-tenant-name>",
       "organization_name": "<your-organization-name>",
       "base_url": "<your-tenant-name>.<sdp-backend-url>",
       "sdp_ui_base_url": "<your-tenant-name>.<sdp-dashboard-url>",
       "owner_email": "<owner-email>",
       "owner_first_name": "<owner-first-name>",
       "owner_last_name": "<owner-last-name>",
       "distribution_account_type": "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT"
     }'
     ```

### Data Migration

1. **Export Data**: - Dump the data from the existing single-tenant instance:
 ```bash pg_dump -U <db-user> -h <db-host> -p <db-port> <db-name> > single_tenant_data.sql ``` 

2. **Create New Tenant**: - Ensure the new tenant is created using the Admin API as described in the New Tenant Provisioning section. 

3. **Restore Data**: - Import the dumped data into the new tenant schema: 
```bash psql -U <db-user> -h <db-host> -p <db-port> <db-name> -f single_tenant_data.sql ``` 

4. **Update References**: - Update all references from the old single-tenant schema to the new tenant schema: 
```sql DO $$ DECLARE r RECORD; BEGIN FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP EXECUTE 'ALTER TABLE ' || quote_ident(r.tablename) || ' SET SCHEMA new_tenant_schema'; END LOOP; END $$; ``` 

5. **Verify Data Migration**: - Verify that all data has been migrated correctly by checking the new tenant schema and ensuring all tables and data are intact.

## Summary

This simplified guide outlines the necessary steps to migrate from SDP Aid (Python) to SDP Single-tenant (v1.x) and then to V2.1.x, focusing on critical actions and configurations. This should make the migration process more straightforward and efficient.
