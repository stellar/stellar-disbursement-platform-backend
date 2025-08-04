# SOROBAN envs that are sourced by stellar-cli automatically:
export STELLAR_RPC_URL=https://soroban-testnet.stellar.org:443
export STELLAR_NETWORK_PASSPHRASE=Test SDF Network ; September 2015
# export STELLAR_RPC_HEADERS=X-SOROBAN-ENVIRONMENT:testnet
export SOURCE_ACCOUNT=default # This is the stellar-cli source account that we set as the recovery account

# Envs used for the smart wallet contract
export PUBLIC_KEY_BYTES=040efb310ac22bd982d0684ce173159c00636de84b0b2868eb7fdff4ca19154d39e0a9af9de6e5fbd8037efe932ef0245ebb1299e14bc3b0babcd77764cacce28a
# export CONTRACT_ID=CCXWEY7OYXSXZEJFG2QRVGHDS5C5PIOCDS5TFYUKHUDSTQBWPDIDMEWI

fmt:
	cargo fmt --all

clean:
	cargo clean

build: fmt
	stellar contract build
	@ls -l ../target/wasm32v1-none/release/smart_wallet.wasm

test:
	cargo test

deploy: clean build
	stellar contract deploy \
	-s default \
	--wasm ../target/wasm32v1-none/release/smart_wallet.wasm \
	-- \
	--admin $(shell stellar keys public-key $(SOURCE_ACCOUNT)) \
	--public_key $(PUBLIC_KEY_BYTES)

