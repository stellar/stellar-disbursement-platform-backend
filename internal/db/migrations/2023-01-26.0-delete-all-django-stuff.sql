-- This migration dumps all django-related stuff that was in the database of the SDP v1.


-- +migrate Up

DROP TABLE IF EXISTS public.django_session CASCADE;
DROP TABLE IF EXISTS public.django_migrations CASCADE;
DROP TABLE IF EXISTS public.django_admin_log CASCADE;
DROP TABLE IF EXISTS public.auth_user_user_permissions CASCADE;
DROP TABLE IF EXISTS public.auth_user_groups CASCADE;
DROP TABLE IF EXISTS public.auth_user CASCADE;
DROP TABLE IF EXISTS public.auth_group_permissions CASCADE;
DROP TABLE IF EXISTS public.auth_permission CASCADE;
DROP TABLE IF EXISTS public.django_content_type CASCADE;
DROP TABLE IF EXISTS public.auth_group CASCADE;
DROP TABLE IF EXISTS public.otp_static_staticdevice CASCADE;
DROP TABLE IF EXISTS public.otp_static_statictoken CASCADE;
DROP TABLE IF EXISTS public.otp_totp_totpdevice CASCADE;
DROP TABLE IF EXISTS public.otp_yubikey_remoteyubikeydevice CASCADE;
DROP TABLE IF EXISTS public.otp_yubikey_validationservice CASCADE;
DROP TABLE IF EXISTS public.otp_yubikey_yubikeydevice CASCADE;
DROP TABLE IF EXISTS public.two_factor_phonedevice CASCADE;

DROP SEQUENCE IF EXISTS public.otp_static_staticdevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS public.otp_static_statictoken_id_seq CASCADE;
DROP SEQUENCE IF EXISTS public.otp_totp_totpdevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS public.otp_yubikey_remoteyubikeydevice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS public.otp_yubikey_validationservice_id_seq CASCADE;
DROP SEQUENCE IF EXISTS public.otp_yubikey_yubikeydevice_id_seq CASCADE;


-- +migrate Down

