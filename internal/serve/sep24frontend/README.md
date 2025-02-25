# SEP-24 Interactive Deposit Application

This is the frontend application for the Stellar SEP-24 interactive deposit flow. This flow 
allows receivers to KYC in order to register their wallets and receive payments from an SDP.
 

## Development

### Running the app directly

To run the app in development mode:

```bash
# Install dependencies
yarn

# Start development server
yarn dev
```

This will start the Vite development server, typically on http://localhost:5173/wallet-registration-fe/

### Building and running with the Go server

1. Build the frontend application:

```bash
# Install dependencies if you haven't already
yarn

# Build the application
yarn build
```

2. Run the Go server:

```bash
go run main.go serve
```

The application will be available at the path configured in your Go server, usually at `/wallet-registration-fe/`.

## Important Note

**After making any changes to the frontend application:**

1. Rebuild the application with `yarn build`
2. Commit the updated `dist` folder to the repository

This is necessary because the Go server serves the pre-built application.