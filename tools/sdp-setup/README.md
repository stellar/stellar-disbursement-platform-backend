# SDP Setup Wizard

A simple interactive wizard to configure the Stellar Disbursement Platform.

## Usage

From the project root:
```bash
make setup         # interactive wizard
```

Or directly:
```bash
go run tools/sdp-setup/main.go
```

## What it does

1. **Configuration Selection**: Choose from existing configurations or create a new one
2. **Network Selection**: Choose between testnet (development) or pubnet (mainnet)
3. **Account Setup**: Generate new Stellar accounts or enter existing ones
4. **Environment Generation**: Creates a properly configured `.env` file with all necessary variables
5. **Launch Option**: Optionally launch the environment immediately

## Notes

- Writes `.env` files into `dev/` directory (e.g., `dev/.env`, `dev/.env.testnet`, `dev/.env.mainnet`)
- On testnet, the wizard can fund the distribution account with XLM+USDC automatically
- Interactive prompts guide you through all configuration choices
- Safety confirmations for mainnet deployments to prevent accidental use of real funds

## Features

- Automatic account generation with testnet funding
- Safety warnings for mainnet deployments
- Database separation between testnet and mainnet
- Security enforcement (MFA required for mainnet)
- Smart defaults for all configuration values

The wizard generates a complete `.env` file that works with the existing Docker Compose setup.
