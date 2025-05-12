#![cfg(test)]

extern crate std;

use crate::webauthn::{
    WebAuthnCredential, AUTH_DATA_FLAG_OFFSET, AUTH_DATA_FLAG_UP, AUTH_DATA_FLAG_UV,
    ENCODED_CHALLENGE_LEN,
};

use soroban_sdk::{
    testutils::{Address as _, BytesN as _},
    vec, BytesN, IntoVal,
};
use std::string::ToString;

use super::*;
use p256::ecdsa::{signature::SignerMut, SigningKey, VerifyingKey};
use rand_core::OsRng;
use soroban_sdk::{Address, Bytes};

fn generate_test_p256_keypair(env: Env) -> (BytesN<65>, SigningKey) {
    let signing_key = SigningKey::random(&mut OsRng);
    let verifying_key = VerifyingKey::from(&signing_key);

    let point = verifying_key.to_encoded_point(false);
    let point_bytes = point.as_bytes();

    let public_key = BytesN::from_array(&env, point_bytes.try_into().unwrap());

    (public_key, signing_key)
}

fn sign(env: Env, challenge_hash: &[u8; 32], signing_key: &mut SigningKey) -> WebAuthnCredential {
    let mut authenticator_data = Bytes::from_slice(&env, &[0; 37]);

    // Fill in RP ID Hash. It's not verified by the contract.
    for i in 0..32 {
        authenticator_data.set(i, i as u8);
    }

    // Set flags: User Present (UP) and User Verified (UV)s
    authenticator_data.set(AUTH_DATA_FLAG_OFFSET, AUTH_DATA_FLAG_UP | AUTH_DATA_FLAG_UV);

    // Create the challenge string
    let mut expected_challenge_buffer = [0_u8; ENCODED_CHALLENGE_LEN as usize];
    base64_url::encode(&mut expected_challenge_buffer, &challenge_hash[0..32]);
    let challenge_str = std::str::from_utf8(&expected_challenge_buffer).unwrap();

    let client_data_json = std::format!(
        r#"{{"type":"webauthn.get","challenge":"{}","origin":"https://example.com"}}"#,
        challenge_str
    );

    // Create the client data hash
    let client_data_json_bytes = client_data_json.as_bytes();
    let client_data_hash = env
        .crypto()
        .sha256(&Bytes::from_slice(&env, client_data_json_bytes));

    let mut message = authenticator_data.clone();
    message.extend_from_slice(&client_data_hash.to_array());

    let mut message_std_vec = std::vec::Vec::with_capacity(message.len() as usize);
    for i in 0..message.len() {
        message_std_vec.push(message.get(i).unwrap());
    }

    // Sign the message (authenticator data + client data hash)
    let signature_object: p256::ecdsa::Signature = signing_key.sign(&message_std_vec);
    let normalized_signature = signature_object.normalize_s();

    let r_bytes = signature_object.r().to_bytes();
    let s_bytes = match normalized_signature {
        Some(normalized) => normalized.s().to_bytes(),
        None => signature_object.s().to_bytes(),
    };

    let mut raw_signature_bytes = [0u8; 64];
    raw_signature_bytes[0..32].copy_from_slice(r_bytes.as_slice());
    raw_signature_bytes[32..64].copy_from_slice(s_bytes.as_slice());

    WebAuthnCredential {
        client_data_json: Bytes::from_slice(&env, client_data_json_bytes),
        authenticator_data,
        signature: BytesN::from_array(&env, &raw_signature_bytes),
    }
}

#[test]
fn test_validate_signature() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env
        .crypto()
        .sha256(&Bytes::from_array(&env, &payload.to_array()));

    let credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    )
    .unwrap();
}

#[test]
fn test_webauthn_invalid_type() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    let original_challenge_str = {
        let mut temp_challenge_buf = [0u8; ENCODED_CHALLENGE_LEN as usize];
        base64_url::encode(&mut temp_challenge_buf, &payload_hash.to_array());
        std::str::from_utf8(&temp_challenge_buf)
            .unwrap()
            .to_string()
    };

    let invalid_type_json_str = std::format!(
        r#"{{"type":"webauthn.create","challenge":"{}","origin":"https://example.com"}}"#,
        original_challenge_str
    );
    credential.client_data_json = Bytes::from_slice(&env, invalid_type_json_str.as_bytes());

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );

    assert_eq!(result, Err(Ok(AccountContractError::WebAuthnInvalidType)));
}

#[test]
fn test_webauthn_client_data_duplicate_fields() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    let original_challenge_str = {
        let mut temp_challenge_buf = [0u8; ENCODED_CHALLENGE_LEN as usize];
        base64_url::encode(&mut temp_challenge_buf, &payload_hash.to_array());
        std::str::from_utf8(&temp_challenge_buf)
            .unwrap()
            .to_string()
    };

    let invalid_type_json_str = std::format!(
        r#"{{"type":"webauthn.get","challenge":"{}", "challenge":"{}", "origin":"https://example.com"}}"#,
        original_challenge_str,
        original_challenge_str
    );
    credential.client_data_json = Bytes::from_slice(&env, invalid_type_json_str.as_bytes());

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );

    assert_eq!(
        result,
        Err(Ok(AccountContractError::WebAuthnInvalidClientData))
    );
}

#[test]
fn test_webauthn_user_not_present() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    // Clear the User Present flag (UP - bit 0)
    let mut auth_data_vec = std::vec::Vec::new();
    for i in 0..credential.authenticator_data.len() {
        auth_data_vec.push(credential.authenticator_data.get(i).unwrap());
    }
    auth_data_vec[AUTH_DATA_FLAG_OFFSET as usize] &= !AUTH_DATA_FLAG_UP;
    credential.authenticator_data = Bytes::from_slice(&env, &auth_data_vec);

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );
    assert_eq!(
        result,
        Err(Ok(AccountContractError::WebAuthnUserNotPresent))
    );
}

#[test]
fn test_webauthn_user_not_verified() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    // Clear the User Verified flag (UV - bit 2)
    let mut auth_data_vec = std::vec::Vec::new();
    for i in 0..credential.authenticator_data.len() {
        auth_data_vec.push(credential.authenticator_data.get(i).unwrap());
    }
    auth_data_vec[AUTH_DATA_FLAG_OFFSET as usize] &= !AUTH_DATA_FLAG_UV;
    credential.authenticator_data = Bytes::from_slice(&env, &auth_data_vec);

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );
    assert_eq!(
        result,
        Err(Ok(AccountContractError::WebAuthnUserNotVerified))
    );
}

#[test]
fn test_webauthn_invalid_challenge_content() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload_sign: BytesN<32> = BytesN::random(&env);
    let payload_hash_sign = env.crypto().sha256(&payload_sign.clone().into());

    let credential = sign(env.clone(), &payload_hash_sign.to_array(), &mut signing_key);

    let different_payload: BytesN<32> = BytesN::random(&env);
    let different_payload_hash = env.crypto().sha256(&different_payload.clone().into());

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &different_payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );
    assert_eq!(
        result,
        Err(Ok(AccountContractError::WebAuthnInvalidChallenge))
    );
}

#[test]
fn test_webauthn_invalid_challenge_length_in_client_data() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    let original_challenge_str = {
        let mut temp_challenge_buf = [0u8; ENCODED_CHALLENGE_LEN as usize];
        base64_url::encode(&mut temp_challenge_buf, &payload_hash.to_array());
        std::str::from_utf8(&temp_challenge_buf)
            .unwrap()
            .to_string()
    };

    let truncated_challenge_str = &original_challenge_str[0..(ENCODED_CHALLENGE_LEN - 1) as usize];
    let bad_client_data_json_str = std::format!(
        r#"{{"type":"webauthn.get","challenge":"{}","origin":"https://example.com"}}"#,
        truncated_challenge_str
    );
    std::dbg!(&bad_client_data_json_str);

    credential.client_data_json = Bytes::from_slice(&env, bad_client_data_json_str.as_bytes());

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );

    assert_eq!(
        result,
        Err(Ok(AccountContractError::WebAuthnInvalidChallenge))
    );
}

#[test]
fn test_webauthn_tampered_signature() {
    let env = Env::default();

    let (public_key, mut signing_key) = generate_test_p256_keypair(env.clone());

    let admin = Address::generate(&env);
    let args = (admin, public_key.clone());
    let contract_address = env.register(AccountContract {}, args);

    let payload: BytesN<32> = BytesN::random(&env);
    let payload_hash = env.crypto().sha256(&payload.clone().into());

    let mut credential = sign(env.clone(), &payload_hash.to_array(), &mut signing_key);

    // Tamper with the signature
    let mut sig_bytes = credential.signature.to_array();
    sig_bytes[0] = sig_bytes[0].wrapping_add(1);
    credential.signature = BytesN::from_array(&env, &sig_bytes);

    let result = env.try_invoke_contract_check_auth::<AccountContractError>(
        &contract_address,
        &BytesN::from_array(&env, &payload_hash.to_array()),
        credential.into_val(&env),
        &vec![&env],
    );

    assert!(result.is_err());
}
