# Stellar Disbursement Platform - Helm Chart

## Table of Contents

- [Stellar Disbursement Platform - Helm Chart](#stellar-disbursement-platform---helm-chart)
  - [Table of Contents](#table-of-contents)
  - [Installation](#installation)
  - [Local Development Cheatsheet](#local-development-cheatsheet)
    - [Using Ingress and a Local TLS Certificate](#using-ingress-and-a-local-tls-certificate)
    - [Creating local secrets for the deployments](#creating-local-secrets-for-the-deployments)

## Installation

```bash
helm install {release-name-here} ./sdp
```

Likewise, to uninstall it you can run:

```bash
helm uninstall {release-name-here}
```

And if you want to upgrade a version that's currently deployed, you can do the following:

```bash
helm upgrade {release-name-here} ./sdp
```

## Local Development Cheatsheet

For debugging purposes, it's sometimes useful to render the templates locally. To do so, you can execute:

```bash
helm template --release-name {release-name-here} -f values.yaml --debug .
```

If you want to deploy this locally, you can enable kubernetes on docker-desktop. Some useful commands:

### Using Ingress and a Local TLS Certificate

To create a self-signed TLS certificate for local development purposes with both `sdp.localhost.com` and `ap.localhost.com` as endpoints, follow these steps (you only need to do it once):

1. Install `ingress-nginx`:

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
kubectl create namespace ingress-nginx
helm install ingress-nginx ingress-nginx/ingress-nginx --namespace=ingress-nginx
```


2. Create a `openssl.cnf` configuration file:

Create a new file named `openssl.cnf` with the following content, which includes both endpoints as subject alternative names (SANs):

```bash
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = localhost

[v3_req]
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = sdp.localhost.com
DNS.2 = ap.localhost.com
```

3. Generate the self-signed certificate and key:
Run the following command to generate a self-signed certificate (`tls.crt`) and private key (`tls.key`) using the configuration file:

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout tls.key -out tls.crt -config openssl.cnf
```

This command will create a certificate valid for 365 days with a 2048-bit RSA key.

4. Create a Kubernetes Secret with the TLS certificate and key:

```bash
kubectl create secret tls stellar-disbursement-platform-backend-tls-cert --key tls.key --cert tls.crt --namespace=stellar-disbursement-platform
```

Replace `myapp-tls` with a descriptive name for your secret.

5. Update your Ingress configuration to use the TLS secret:

Add the `tls` section to your Ingress manifest, referencing the secret you created in step 3:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: stellar-disbursement-platform-backend
  namespace: stellar-disbursement-platform
  # ... other metadata ...
spec:
  ingressClassName: nginx  # <---- This is important!
  tls:
  - hosts:
    - sdp.localhost.com
    - ap.localhost.com
    secretName: myapp-tls
  rules:
    # ... existing rules ...
```

Reapply the Ingress manifest:

```bash
kubectl apply -f <your-ingress-file>.yaml
```

6. Update your `/etc/hosts` file by adding:

```bash
# SDP + Anchor Platform:
127.0.0.1 sdp.localhost.com
127.0.0.1 ap.localhost.com
```

🎉 Now, you should be able to access your services at `https://sdp.localhost.com` and `https://ap.localhost.com`. Keep in mind that browsers will display a security warning when accessing your site due to the use of a self-signed certificate. You can add an exception for your local domains to trust the self-signed certificate.

### Creating local secrets for the deployments

To create the secrets containing the env vars required for the deployments, simply create a .env file with the desired values, then run:
  
```bash
kubectl create secret generic <my-secret> --from-env-file=<my-secret-values.env> --namespace=<your-namespace>
```
