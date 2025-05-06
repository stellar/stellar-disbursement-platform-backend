#![cfg(test)]

extern crate std;

use crate::webauthn::{verify, WebAuthnCredential};

use super::*;
use p256::ecdsa::{signature::SignerMut, SigningKey, VerifyingKey};
use rand_core::OsRng;
use soroban_sdk::Bytes;

fn generate_test_p256_keypair(env: Env) -> (BytesN<65>, SigningKey) {
    let signing_key = SigningKey::random(&mut OsRng);
    let verifying_key = VerifyingKey::from(&signing_key);

    let point = verifying_key.to_encoded_point(false);
    let point_bytes = point.as_bytes();

    let public_key = BytesN::from_array(&env, point_bytes.try_into().unwrap());

    (public_key, signing_key)
}

#[test]
fn test() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let challenge = Bytes::from_slice(&env, &[0; 32]);
    let signature_payload = env.crypto().sha256(&challenge);

    let mut auth_data = Bytes::from_slice(&env, &[0; 37]);
    // Fill in the rpIdHash
    for i in 0..32 {
        auth_data.set(i, i as u8);
    }
    auth_data.set(32, 0x01 | 0x04);

    let mut expected_challenge = [0_u8; 43];
    base64_url::encode(&mut expected_challenge, &signature_payload.to_array());
    let challenge_str = std::str::from_utf8(&expected_challenge).unwrap();

    let client_data_json = std::format!(
        r#"{{"type":"webauthn.get","challenge":"{}","origin":"https://example.com"}}"#,
        challenge_str
    );

    let client_data_json_bytes = client_data_json.as_bytes();
    let client_data_hash = env
        .crypto()
        .sha256(&Bytes::from_slice(&env, client_data_json_bytes));

    let mut message = auth_data.clone();
    message.extend_from_slice(&client_data_hash.to_array());

    let mut message_std_vec = std::vec::Vec::with_capacity(message.len() as usize);
    for i in 0..message.len() {
        message_std_vec.push(message.get(i).unwrap());
    }

    let signature_object: p256::ecdsa::Signature = signing_key.sign(&message_std_vec);

    let r_bytes = signature_object.r().to_bytes();
    let s_bytes = signature_object.s().to_bytes();

    let mut raw_signature_bytes = [0u8; 64];
    raw_signature_bytes[0..32].copy_from_slice(r_bytes.as_slice());
    raw_signature_bytes[32..64].copy_from_slice(s_bytes.as_slice());

    // Correctly determine the indices
    let type_index = client_data_json.find("webauthn.get").unwrap() as u32;
    let challenge_index = client_data_json.find(challenge_str).unwrap() as u32;

    let credential = WebAuthnCredential {
        client_data_json: Bytes::from_slice(&env, client_data_json_bytes),
        authenticator_data: auth_data,
        type_index,
        challenge_index,
        signature: BytesN::from_array(&env, &raw_signature_bytes),
    };

    verify(&env, &signature_payload, &public_key, &credential);

    // let signed_credential = WebAuthnSignedCredential {
    //     public_key,
    //     credential,
    // };
}
