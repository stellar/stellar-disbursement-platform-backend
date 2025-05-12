use soroban_sdk::{contracttype, crypto::Hash, panic_with_error, Bytes, BytesN, Env};

use crate::{base64_url, AccountContractError};

/// The WebAuthn type for the get operation.
pub(crate) const WEBAUTHN_TYPE_GET: &str = "webauthn.get";

/// Authenticator data flag offset. It appears after the RP ID hash in the authenticator data.
pub(crate) const AUTH_DATA_FLAG_OFFSET: u32 = 32;
/// Authenticator data flags for user presence
pub(crate) const AUTH_DATA_FLAG_UP: u8 = 0x01;
/// Authenticator data flags for user verification
pub(crate) const AUTH_DATA_FLAG_UV: u8 = 0x04;

/// Length of the encoded challenge in the client data JSON.
pub(crate) const ENCODED_CHALLENGE_LEN: u32 = 43;

/// Max length of the client data JSON in bytes.
///
/// #### Explanation of the length:
///
/// - `type`: ~20 bytes (`"type":"webauthn.get"`).
///
/// - `challenge`: ~58 bytes (`"challenge":"<base64url_32_byte_challenge>",`).
///
/// - `origin`: ~100-200 bytes (`"origin":"https://example.com",`)
///
/// - `crossOrigin`: ~20 bytes (`"crossOrigin":false,`).
///
/// Total length: ~298 bytes.
///
/// This is a conservative estimate, as the actual length may vary based on the specific values used.
/// The maximum length is set to 1024 bytes to accommodate any additional fields or whitespace.
pub(crate) const MAX_CLIENT_DATA_JSON_LEN: usize = 1024;

/// The Client Data JSON structure.
#[derive(serde::Deserialize, Clone, Debug, PartialEq, PartialOrd)]
struct ClientDataJson<'a> {
    /// The type of the WebAuthn operation.
    pub r#type: &'a str,
    /// The challenge used in the WebAuthn operation.
    pub challenge: &'a str,
}

/// A WebAuthn credential.
#[derive(Clone, Debug, PartialEq, PartialOrd)]
#[contracttype]
pub struct WebAuthnCredential {
    /// The authenticator data is a base64url encoded string.
    pub authenticator_data: Bytes,
    /// The client data JSON is a base64url encoded string.
    pub client_data_json: Bytes,
    /// The signature over the authenticator data and client data JSON hash.
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
/// * `credential` - The WebAuthn credential containing the signature and other data.
/// * `public_key` - The public key used for signature verification.
///
/// # Panics
///
/// This function will panic if any of the checks fail.
pub fn verify(
    env: &Env,
    signature_payload: &Hash<32>,
    credential: &WebAuthnCredential,
    public_key: &BytesN<65>,
) {
    // Parse the client data JSON
    let client_data_json = credential
        .client_data_json
        .to_buffer::<MAX_CLIENT_DATA_JSON_LEN>();
    let client_data_json = client_data_json.as_slice();

    let (client_data, _): (ClientDataJson, _) = serde_json_core::de::from_slice(client_data_json)
        .unwrap_or_else(|_| {
            panic_with_error!(env, AccountContractError::WebAuthnInvalidClientData);
        });

    // 1. Verify the Webauthn type
    if client_data.r#type != WEBAUTHN_TYPE_GET {
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
    let mut expected_challenge = [0_u8; ENCODED_CHALLENGE_LEN as usize];
    base64_url::encode(&mut expected_challenge, &signature_payload.to_array());

    if client_data.challenge.as_bytes() != expected_challenge {
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
