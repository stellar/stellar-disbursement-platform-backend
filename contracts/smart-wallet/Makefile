# SOROBAN envs that are sourced by stellar-cli automatically:
export STELLAR_RPC_URL=https://soroban-testnet.stellar.org:443
export STELLAR_NETWORK_PASSPHRASE=Test SDF Network ; September 2015
# export STELLAR_RPC_HEADERS=X-SOROBAN-ENVIRONMENT:testnet
export SOURCE_ACCOUNT=multisig-sdp # This is the stellar-cli source account that we set as the recovery account

# Envs used for the smart wallet contract
export PUBLIC_KEY_BYTES1=040efb310ac22bd982d0684ce173159c00636de84b0b2868eb7fdff4ca19154d39e0a9af9de6e5fbd8037efe932ef0245ebb1299e14bc3b0babcd77764cacce28a
export CONTRACT_ID=CASKRGJVAFSZ4HPZ7446NO64LCDZ673MDCYCM2YWTAEOQNC52R62DAG5
export PUBLIC_KEY_BYTES2=04226cdb3a8732cb7fe859496c82d28a5653ac2ec2197448e1dc98b824b3696d604d50c6c01c6d5c9b54aa4a42c98cdc822d5fd46048437ee0f4ffc0a8db6b996c

fmt:
	cargo fmt --all

clean:
	cargo clean

build: fmt
	stellar contract build
	@ls -l ../target/wasm32v1-none/release/smart_wallet.wasm

test:
	cargo test

upload:
	stellar contract upload \
	-s $(SOURCE_ACCOUNT) \
	--wasm ../target/wasm32v1-none/release/smart_wallet.wasm

deploy: clean build
	@echo "==========\n⚠️ DISCLAIMER: This command is for TEST purposes only. Once deployed, the contract will only pass authentication if the client has the secret key and can produce the appropriate signature.\n=========="
	stellar contract deploy \
	-s $(SOURCE_ACCOUNT) \
	--wasm ../target/wasm32v1-none/release/smart_wallet.wasm \
	-- \
	--recovery $(shell stellar keys public-key $(SOURCE_ACCOUNT)) \
	--public_key $(PUBLIC_KEY_BYTES1)

rotate-signer:
	stellar contract invoke -s multisig \
	-- rotate_signer \
	--new_signer $(PUBLIC_KEY_BYTES2)
