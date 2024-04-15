-- This migration dumps all django-related stuff that was in the database of the SDP v1.


-- +migrate Up

DROP TABLE IF EXISTS django_session CASCADE;
DROP TABLE IF EXISTS django_migrations CASCADE;
DROP TABLE IF EXISTS django_admin_log CASCADE;
DROP TABLE IF EXISTS auth_user_user_permissions CASCADE;
DROP TABLE IF EXISTS auth_user_groups CASCADE;
DROP TABLE IF EXISTS auth_user CASCADE;
DROP TABLE IF EXISTS auth_group_permissions CASCADE;
DROP TABLE IF EXISTS auth_permission CASCADE;
DROP TABLE IF EXISTS django_content_type CASCADE;
DROP TABLE IF EXISTS auth_group CASCADE;
DROP TABLE IF EXISTS otp_static_staticdevice CASCADE;
DROP TABLE IF EXISTS otp_static_statictoken CASCADE;
DROP TABLE IF EXISTS otp_totp_totpdevice CASCADE;
DROP TABLE IF EXISTS otp_yubikey_remoteyubikeydevice CASCADE;
DROP TABLE IF EXISTS otp_yubikey_validationservice CASCADE;
DROP TABLE IF EXISTS otp_yubikey_yubikeydevice CASCADE;
DROP TABLE IF EXISTS two_factor_phonedevice CASCADE;

DROP SEQUENCE IF EXISTS otp_static_staticdevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS otp_static_statictoken_id_seq CASCADE;
DROP SEQUENCE IF EXISTS otp_totp_totpdevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS otp_yubikey_remoteyubikeydevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS otp_yubikey_validationservice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS otp_yubikey_yubikeydevice_id_seq CASCADE;


-- +migrate Down

