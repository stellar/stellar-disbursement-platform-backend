#![no_std]

use soroban_sdk::{
    auth::{Context, CustomAccountInterface},
    contract, contracterror, contractimpl, contracttype,
    crypto::Hash,
    Address, Bytes, BytesN, Env, Vec,
};

mod base64_url;
mod webauthn;

#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracttype]
pub enum DataKey {
    Admin,
    Signer(Bytes),
}

#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracterror]
pub enum AccountContractError {
    SingleSignatureRequired = 0,
    UnknownSigner = 1,
    WebAuthnInvalidType = 2,
    WebAuthnUserNotPresent = 3,
    WebAuthnUserNotVerified = 4,
    WebAuthnInvalidChallenge = 5,
}

#[contract]
pub struct AccountContract;

#[contractimpl]
impl AccountContract {
    pub fn __constructor(env: Env, admin: Address, credential_id: Bytes, public_key: BytesN<65>) {
        env.storage().instance().set(&DataKey::Admin, &admin);
        env.storage()
            .instance()
            .set(&DataKey::Signer(credential_id), &public_key);
    }

    pub fn upgrade(env: Env, new_wasm_hash: BytesN<32>) {
        let admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        admin.require_auth();

        env.deployer().update_current_contract_wasm(new_wasm_hash);
    }
}

#[contractimpl]
impl CustomAccountInterface for AccountContract {
    type Error = AccountContractError;
    type Signature = Vec<webauthn::WebAuthnCredential>;

    fn __check_auth(
        env: Env,
        signature_payload: Hash<32>,
        signatures: Self::Signature,
        _auth_contexts: Vec<Context>,
    ) -> Result<(), Self::Error> {
        if signatures.len() != 1 {
            return Err(AccountContractError::SingleSignatureRequired);
        }

        let signature = signatures.get(0).unwrap();

        let public_key = env
            .storage()
            .instance()
            .get::<_, BytesN<65>>(&DataKey::Signer(signature.clone().credential_id))
            .ok_or(AccountContractError::UnknownSigner)?;

        webauthn::verify(&env, &signature_payload, &signature, &public_key);

        Ok(())
    }
}

mod test;
