#![no_std]

use soroban_sdk::{
    auth::{Context, CustomAccountInterface},
    contract, contracterror, contractimpl, contracttype,
    crypto::Hash,
    Address, BytesN, Env, Vec,
};

mod base64_url;
mod webauthn;

#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracttype]
pub enum DataKey {
    Admin,
    Signer,
    Recovery,
}

#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracterror]
pub enum AccountContractError {
    MissingSigner = 0,
    WebAuthnInvalidClientData = 1,
    WebAuthnInvalidType = 2,
    WebAuthnUserNotPresent = 3,
    WebAuthnUserNotVerified = 4,
    WebAuthnInvalidChallenge = 5,
}

#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracterror]
pub enum RecoveryError {
    RecoveryNotSet = 1000,
}

pub trait Recovery {
    fn rotate_signer(env: Env, new_signer: BytesN<65>) -> Result<(), RecoveryError>;
}

#[contract]
pub struct AccountContract;

#[contractimpl]
impl AccountContract {
    pub fn __constructor(env: Env, public_key: BytesN<65>, recovery: Address) {
        env.storage().instance().set(&DataKey::Signer, &public_key);
        env.storage().instance().set(&DataKey::Recovery, &recovery);
    }
}

#[contractimpl]
impl Recovery for AccountContract {
    fn rotate_signer(env: Env, new_signer: BytesN<65>) -> Result<(), RecoveryError> {
        let recovery = env
            .storage()
            .instance()
            .get::<_, Address>(&DataKey::Recovery)
            .ok_or(RecoveryError::RecoveryNotSet)?;
        recovery.require_auth();

        env.storage().instance().set(&DataKey::Signer, &new_signer);

        Ok(())
    }
}

#[contractimpl]
impl CustomAccountInterface for AccountContract {
    type Error = AccountContractError;
    type Signature = webauthn::WebAuthnCredential;

    fn __check_auth(
        env: Env,
        signature_payload: Hash<32>,
        signatures: Self::Signature,
        _auth_contexts: Vec<Context>,
    ) -> Result<(), Self::Error> {
        let public_key = env
            .storage()
            .instance()
            .get::<_, BytesN<65>>(&DataKey::Signer)
            .ok_or(AccountContractError::MissingSigner)?;

        webauthn::verify(&env, &signature_payload, &signatures, &public_key);

        Ok(())
    }
}

mod test;
