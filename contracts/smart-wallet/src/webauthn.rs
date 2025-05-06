use soroban_sdk::{contracttype, crypto::Hash, panic_with_error, Bytes, BytesN, Env};

use crate::{base64_url, AccountContractError};

pub(crate) const AUTH_DATA_FLAG_OFFSET: u32 = 32;
pub(crate) const AUTH_DATA_FLAG_UP: u8 = 0x01;
pub(crate) const AUTH_DATA_FLAG_UV: u8 = 0x04;
pub(crate) const ENCODED_CHALLENGE_LEN: u32 = 43;
pub(crate) const WEBAUTHN_TYPE_GET: &str = "webauthn.get";

/// A WebAuthn credential.
#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracttype]
pub struct WebAuthnCredential {
    pub public_key: BytesN<65>,
    /// The authenticator data is a base64url encoded string.
    pub authenticator_data: Bytes,
    /// The client data JSON is a base64url encoded string.
    pub client_data_json: Bytes,
    /// The type index is the starting index of the type in the client data JSON.
    pub type_index: u32,
    /// The challenge index is the starting index of the challenge in the client data JSON.
    pub challenge_index: u32,
    /// The signature over the authenticator data and client data JSON.
    pub signature: BytesN<64>,
}

/// The `verify` function checks the validity of a WebAuthn signature.
/// 
/// It performs the following checks:
/// 1. Verifies the WebAuthn type.
/// 2. Checks the authenticator data flags.
/// 3. Validates the challenge.
/// 4. Verifies the cryptographic signature.
/// 
/// # Arguments
/// 
/// * `env` - The Soroban environment.
/// * `signature_payload` - The payload used for signature verification.
/// * `public_key` - The public key of the signer.
/// * `credential` - The WebAuthn credential containing the signature and other data.
/// 
/// # Panics
/// 
/// This function will panic if any of the checks fail.
pub fn verify(
    env: &Env,
    signature_payload: &Hash<32>,
    public_key: &BytesN<65>,
    credential: &WebAuthnCredential,
) {
    // 1. Verify the Webauthn type
    const WEBAUTHN_TYPE_GET_LEN: u32 = 12;
    
    let type_slice = credential.client_data_json.slice(
        credential.type_index..(credential.type_index + WEBAUTHN_TYPE_GET_LEN)
    );
    
    let expected_type = Bytes::from_slice(env, WEBAUTHN_TYPE_GET.as_bytes());
    
    if !type_slice.eq(&expected_type) {
        panic_with_error!(env, AccountContractError::WebAuthnInvalidType);
    }

    // 2. Verify the authenticator data flags
    let flags = credential
        .authenticator_data
        .get(AUTH_DATA_FLAG_OFFSET)
        .unwrap();
    
    // Check user presence flag
    if flags & AUTH_DATA_FLAG_UP != AUTH_DATA_FLAG_UP {
        panic_with_error!(env, AccountContractError::WebAuthnUserNotPresent);
    }
    
    // Check user verification flag
    if flags & AUTH_DATA_FLAG_UV != AUTH_DATA_FLAG_UV {
        panic_with_error!(env, AccountContractError::WebAuthnUserNotVerified);
    }

    // 3. Verify the challenge
    let challenge_start = credential.challenge_index;
    let challenge_end = challenge_start + ENCODED_CHALLENGE_LEN;

    if challenge_end > credential.client_data_json.len() {
        panic_with_error!(env, AccountContractError::WebAuthnInvalidChallenge);
    }

    let challenge_slice = credential
        .client_data_json
        .slice(challenge_start..challenge_end);

    if challenge_slice.len() != ENCODED_CHALLENGE_LEN {
        panic_with_error!(env, AccountContractError::WebAuthnInvalidChallenge);
    }

    // Compare actual challenge with expected challenge
    let mut actual_challenge = [0_u8; ENCODED_CHALLENGE_LEN as usize];
    challenge_slice.copy_into_slice(&mut actual_challenge);

    let mut expected_challenge = [0_u8; ENCODED_CHALLENGE_LEN as usize];
    base64_url::encode(&mut expected_challenge, &signature_payload.to_array());

    if expected_challenge != actual_challenge {
        panic_with_error!(env, AccountContractError::WebAuthnInvalidChallenge);
    }

    // 4. Verify the cryptographic signature
    let client_data_hash = env.crypto().sha256(&credential.client_data_json);

    let mut message = credential.authenticator_data.clone();
    message.extend_from_slice(&client_data_hash.to_array());
    let message_hash = env.crypto().sha256(&message);

    env.crypto()
        .secp256r1_verify(public_key, &message_hash, &credential.signature);
}
