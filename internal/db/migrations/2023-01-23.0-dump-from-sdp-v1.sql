-- This migration file is meant to reproduce the database schema from the SDP v1, so we can support users that
-- are on SDP v1 to migrate to SDP v2.

-- +migrate Up


------------------------------------------------- START DJANGO MODELS -------------------------------------------------

-- TABLE: auth_group
CREATE TABLE IF NOT EXISTS public.auth_group (
    id SERIAL PRIMARY KEY,
    name character varying(150) NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_group_name_a6ea08ec_like ON public.auth_group (name varchar_pattern_ops);
ALTER INDEX auth_group_name_a6ea08ec_like RENAME TO auth_group_name_idx;


-- TABLE: django_content_type
CREATE TABLE IF NOT EXISTS public.django_content_type (
    id SERIAL PRIMARY KEY,
    app_label character varying(100) NOT NULL,
    model character varying(100) NOT NULL,
    UNIQUE (app_label, model)
);

INSERT INTO public.django_content_type VALUES (1, 'admin', 'logentry') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (2, 'auth', 'permission') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (3, 'auth', 'group') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (4, 'auth', 'user') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (5, 'contenttypes', 'contenttype') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (6, 'sessions', 'session') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (7, 'payments', 'account') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (8, 'payments', 'disbursement') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (9, 'payments', 'heartbeat') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (10, 'payments', 'payment') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (11, 'payments', 'activation') ON CONFLICT (id) DO NOTHING;
INSERT INTO public.django_content_type VALUES (12, 'payments', 'withdrawal') ON CONFLICT (id) DO NOTHING;


-- TABLE: auth_permission
CREATE TABLE IF NOT EXISTS public.auth_permission (
    id SERIAL PRIMARY KEY,
    name character varying(255) NOT NULL,
    content_type_id integer NOT NULL REFERENCES public.django_content_type (id) DEFERRABLE INITIALLY DEFERRED,
    codename character varying(100) NOT NULL,
    UNIQUE (content_type_id, codename)
);
CREATE INDEX IF NOT EXISTS auth_permission_content_type_id_2f476e4b ON public.auth_permission USING btree (content_type_id);
ALTER INDEX auth_permission_content_type_id_2f476e4b RENAME TO auth_permission_content_type_id_idx;

INSERT INTO public.auth_permission VALUES (1, 'Can add log entry', 1, 'add_logentry') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (2, 'Can change log entry', 1, 'change_logentry') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (3, 'Can delete log entry', 1, 'delete_logentry') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (4, 'Can view log entry', 1, 'view_logentry') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (5, 'Can add permission', 2, 'add_permission') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (6, 'Can change permission', 2, 'change_permission') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (7, 'Can delete permission', 2, 'delete_permission') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (8, 'Can view permission', 2, 'view_permission') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (9, 'Can add group', 3, 'add_group') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (10, 'Can change group', 3, 'change_group') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (11, 'Can delete group', 3, 'delete_group') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (12, 'Can view group', 3, 'view_group') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (13, 'Can add user', 4, 'add_user') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (14, 'Can change user', 4, 'change_user') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (15, 'Can delete user', 4, 'delete_user') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (16, 'Can view user', 4, 'view_user') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (17, 'Can add content type', 5, 'add_contenttype') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (18, 'Can change content type', 5, 'change_contenttype') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (19, 'Can delete content type', 5, 'delete_contenttype') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (20, 'Can view content type', 5, 'view_contenttype') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (21, 'Can add session', 6, 'add_session') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (22, 'Can change session', 6, 'change_session') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (23, 'Can delete session', 6, 'delete_session') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (24, 'Can view session', 6, 'view_session') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (25, 'Can add account', 7, 'add_account') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (26, 'Can change account', 7, 'change_account') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (27, 'Can delete account', 7, 'delete_account') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (28, 'Can view account', 7, 'view_account') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (29, 'Can add disbursement', 8, 'add_disbursement') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (30, 'Can change disbursement', 8, 'change_disbursement') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (31, 'Can delete disbursement', 8, 'delete_disbursement') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (32, 'Can view disbursement', 8, 'view_disbursement') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (33, 'Can add heart beat', 9, 'add_heartbeat') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (34, 'Can change heart beat', 9, 'change_heartbeat') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (35, 'Can delete heart beat', 9, 'delete_heartbeat') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (36, 'Can view heart beat', 9, 'view_heartbeat') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (37, 'Can add payment', 10, 'add_payment') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (38, 'Can change payment', 10, 'change_payment') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (39, 'Can delete payment', 10, 'delete_payment') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (40, 'Can view payment', 10, 'view_payment') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (41, 'Can add activation', 11, 'add_activation') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (42, 'Can change activation', 11, 'change_activation') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (43, 'Can delete activation', 11, 'delete_activation') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (44, 'Can view activation', 11, 'view_activation') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (45, 'Can add withdrawal', 12, 'add_withdrawal') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (46, 'Can change withdrawal', 12, 'change_withdrawal') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (47, 'Can delete withdrawal', 12, 'delete_withdrawal') ON CONFLICT DO NOTHING;
INSERT INTO public.auth_permission VALUES (48, 'Can view withdrawal', 12, 'view_withdrawal') ON CONFLICT DO NOTHING;


-- TABLE: auth_group_permissions
CREATE TABLE IF NOT EXISTS public.auth_group_permissions (
    id BIGSERIAL PRIMARY KEY,
    group_id integer NOT NULL REFERENCES public.auth_group (id) DEFERRABLE INITIALLY DEFERRED,
    permission_id integer NOT NULL REFERENCES public.auth_permission (id) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (group_id, permission_id)
);
CREATE INDEX IF NOT EXISTS auth_group_permissions_group_id_b120cbf9 ON public.auth_group_permissions USING btree (group_id);
ALTER INDEX auth_group_permissions_group_id_b120cbf9 RENAME TO auth_group_permissions_group_id_idx;

CREATE INDEX IF NOT EXISTS auth_group_permissions_permission_id_84c5c92e ON public.auth_group_permissions USING btree (permission_id);
ALTER INDEX auth_group_permissions_permission_id_84c5c92e RENAME TO auth_group_permissions_permission_id_idx;


-- TABLE: auth_user
CREATE TABLE IF NOT EXISTS public.auth_user (
    id SERIAL PRIMARY KEY,
    password character varying(128) NOT NULL,
    last_login timestamp with time zone,
    is_superuser boolean NOT NULL,
    username character varying(150) NOT NULL,
    first_name character varying(150) NOT NULL,
    last_name character varying(150) NOT NULL,
    email character varying(254) NOT NULL,
    is_staff boolean NOT NULL,
    is_active boolean NOT NULL,
    date_joined timestamp with time zone NOT NULL,
    UNIQUE (username)
);
CREATE INDEX IF NOT EXISTS auth_user_username_6821ab7c_like ON public.auth_user USING btree (username varchar_pattern_ops);
ALTER INDEX auth_user_username_6821ab7c_like RENAME TO auth_user_username_idx;


-- TABLE: auth_user_groups
CREATE TABLE IF NOT EXISTS public.auth_user_groups (
    id BIGSERIAL PRIMARY KEY,
    user_id integer NOT NULL REFERENCES public.auth_user (id) DEFERRABLE INITIALLY DEFERRED,
    group_id integer NOT NULL REFERENCES public.auth_group (id) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (user_id, group_id)
);
CREATE INDEX IF NOT EXISTS auth_user_groups_group_id_97559544 ON public.auth_user_groups USING btree (group_id);
ALTER INDEX auth_user_groups_group_id_97559544 RENAME TO auth_user_groups_group_id_idx;

CREATE INDEX IF NOT EXISTS auth_user_groups_user_id_6a12ed8b ON public.auth_user_groups USING btree (user_id);
ALTER INDEX auth_user_groups_user_id_6a12ed8b RENAME TO auth_user_groups_user_id_idx;


-- TABLE: auth_user_user_permissions
CREATE TABLE IF NOT EXISTS public.auth_user_user_permissions (
    id BIGSERIAL PRIMARY KEY,
    user_id integer NOT NULL REFERENCES public.auth_user (id) DEFERRABLE INITIALLY DEFERRED,
    permission_id integer NOT NULL REFERENCES public.auth_permission (id) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (user_id, permission_id)
);
CREATE INDEX IF NOT EXISTS auth_user_user_permissions_permission_id_1fbb5f2c ON public.auth_user_user_permissions USING btree (permission_id);
ALTER INDEX auth_user_user_permissions_permission_id_1fbb5f2c RENAME TO auth_user_user_permissions_permission_id_idx;

CREATE INDEX IF NOT EXISTS auth_user_user_permissions_user_id_a95ead1b ON public.auth_user_user_permissions USING btree (user_id);
ALTER INDEX auth_user_user_permissions_user_id_a95ead1b RENAME TO auth_user_user_permissions_user_id_idx;


-- TABLE: django_admin_log
CREATE TABLE IF NOT EXISTS public.django_admin_log (
    id SERIAL PRIMARY KEY,
    action_time timestamp with time zone NOT NULL,
    object_id text,
    object_repr character varying(200) NOT NULL,
    action_flag smallint NOT NULL,
    change_message text NOT NULL,
    content_type_id integer REFERENCES public.django_content_type(id) DEFERRABLE INITIALLY DEFERRED,
    user_id integer NOT NULL REFERENCES public.auth_user(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT django_admin_log_action_flag_check CHECK ((action_flag >= 0))
);
CREATE INDEX IF NOT EXISTS django_admin_log_content_type_id_c4bce8eb ON public.django_admin_log USING btree (content_type_id);
ALTER INDEX django_admin_log_content_type_id_c4bce8eb RENAME TO django_admin_log_content_type_id_idx;

CREATE INDEX IF NOT EXISTS django_admin_log_user_id_c564eba6 ON public.django_admin_log USING btree (user_id);
ALTER INDEX django_admin_log_user_id_c564eba6 RENAME TO django_admin_log_user_id_idx;


-- TABLE: django_content_type
CREATE TABLE IF NOT EXISTS public.django_migrations (
    id BIGSERIAL PRIMARY KEY,
    app character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    applied timestamp with time zone NOT NULL
);

INSERT INTO public.django_migrations VALUES (1, 'contenttypes', '0001_initial', '2023-01-04 16:05:52.644099-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (2, 'auth', '0001_initial', '2023-01-04 16:05:52.711348-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (3, 'admin', '0001_initial', '2023-01-04 16:05:52.731795-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (4, 'admin', '0002_logentry_remove_auto_add', '2023-01-04 16:05:52.742003-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (5, 'admin', '0003_logentry_add_action_flag_choices', '2023-01-04 16:05:52.752853-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (6, 'contenttypes', '0002_remove_content_type_name', '2023-01-04 16:05:52.770614-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (7, 'auth', '0002_alter_permission_name_max_length', '2023-01-04 16:05:52.780492-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (8, 'auth', '0003_alter_user_email_max_length', '2023-01-04 16:05:52.791342-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (9, 'auth', '0004_alter_user_username_opts', '2023-01-04 16:05:52.802137-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (10, 'auth', '0005_alter_user_last_login_null', '2023-01-04 16:05:52.811967-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (11, 'auth', '0006_require_contenttypes_0002', '2023-01-04 16:05:52.814286-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (12, 'auth', '0007_alter_validators_add_error_messages', '2023-01-04 16:05:52.824612-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (13, 'auth', '0008_alter_user_username_max_length', '2023-01-04 16:05:52.835405-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (14, 'auth', '0009_alter_user_last_name_max_length', '2023-01-04 16:05:52.846608-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (15, 'auth', '0010_alter_group_name_max_length', '2023-01-04 16:05:52.858149-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (16, 'auth', '0011_update_proxy_permissions', '2023-01-04 16:05:52.867694-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (17, 'auth', '0012_alter_user_first_name_max_length', '2023-01-04 16:05:52.879894-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (18, 'payments', '0001_initial', '2023-01-04 16:05:52.947015-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (19, 'payments', '0002_remove_disbursement_requested_by_and_more', '2023-01-04 16:05:52.985496-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (20, 'payments', '0003_remove_disbursement_amount_and_more', '2023-01-04 16:05:53.040818-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (21, 'payments', '0004_account_link_last_sent_at', '2023-01-04 16:05:53.051175-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (22, 'payments', '0005_account_date_of_birth_account_email_and_more', '2023-01-04 16:05:53.079481-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (23, 'payments', '0006_payment_idempotency_key_alter_account_status', '2023-01-04 16:05:53.09219-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (24, 'payments', '0007_rename_hashed_date_of_birth_account_hashed_extra_info_and_more', '2023-01-04 16:05:53.108973-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (25, 'payments', '0008_activation', '2023-01-04 16:05:53.132048-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (26, 'payments', '0009_add_yubikey_validation_service', '2023-01-04 16:05:53.146663-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (27, 'payments', '0010_alter_account_phone_number', '2023-01-04 16:05:53.155158-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (28, 'payments', '0011_alter_payment_status', '2023-01-04 16:05:53.160529-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (29, 'payments', '0012_alter_payment_amount', '2023-01-04 16:05:53.173929-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (30, 'payments', '0013_withdrawal_alter_account_status_and_more', '2023-01-04 16:05:53.211413-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (31, 'payments', '0014_payment_withdrawal_amount_payment_withdrawal_status', '2023-01-04 16:05:53.228509-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (32, 'payments', '0015_rename_stellar_transaction_id_withdrawal_sep24_transaction_id', '2023-01-04 16:05:53.237819-08') ON CONFLICT DO NOTHING;
INSERT INTO public.django_migrations VALUES (33, 'sessions', '0001_initial', '2023-01-04 16:05:53.247677-08') ON CONFLICT DO NOTHING;


-- TABLE: django_session
CREATE TABLE IF NOT EXISTS public.django_session (
    session_key character varying(40) NOT NULL PRIMARY KEY,
    session_data text NOT NULL,
    expire_date timestamp with time zone NOT NULL
);
CREATE INDEX IF NOT EXISTS django_session_expire_date_a5c62663 ON public.django_session USING btree (expire_date);
ALTER INDEX django_session_expire_date_a5c62663 RENAME TO django_session_expire_date_idx;

CREATE INDEX IF NOT EXISTS django_session_session_key_c0390e0f_like ON public.django_session USING btree (session_key varchar_pattern_ops);
ALTER INDEX django_session_session_key_c0390e0f_like RENAME TO django_session_session_key_idx;

------------------------------------------------- FINISH DJANGO MODELS -------------------------------------------------


------------------------------------------------- START OF SDP MODELS -------------------------------------------------

-- TABLE: receiver (previously known as payments_account)
CREATE TABLE IF NOT EXISTS public.payments_account (
    id character varying(64) NOT NULL PRIMARY KEY,
    public_key character varying(128),
    registered_at timestamp with time zone NOT NULL,
    phone_number character varying(32) NOT NULL,
    public_key_registered_at timestamp with time zone,
    status character varying(32) NOT NULL,
    link_last_sent_at timestamp with time zone,
    email character varying(254),
    email_registered_at timestamp with time zone,
    hashed_extra_info character varying(64) NOT NULL,
    hashed_phone_number character varying(64) NOT NULL,
    extra_info character varying(64) NOT NULL,
    UNIQUE (phone_number)
);
CREATE INDEX IF NOT EXISTS payments_ac_hashed__f9420c_idx ON public.payments_account USING btree (hashed_phone_number, hashed_extra_info);
ALTER INDEX payments_ac_hashed__f9420c_idx RENAME TO receiver_hashed_phone_and_hashed_extra_info_idx;

CREATE INDEX IF NOT EXISTS payments_account_phone_number_221a9f17_like ON public.payments_account USING btree (phone_number varchar_pattern_ops);
ALTER INDEX payments_account_phone_number_221a9f17_like RENAME TO receiver_phone_number_idx;

CREATE INDEX IF NOT EXISTS payments_ac_registe_104353_idx ON public.payments_account USING btree (registered_at DESC);
ALTER INDEX payments_ac_registe_104353_idx RENAME TO receiver_registered_at_idx;

ALTER TABLE payments_account RENAME TO receivers;


-- TABLE: on_off_switch (previously known as payments_activation)
CREATE TABLE IF NOT EXISTS public.payments_activation (
    id BIGSERIAL PRIMARY KEY,
    is_active boolean NOT NULL,
    last_set_at timestamp with time zone NOT NULL DEFAULT NOW()
);
INSERT INTO public.payments_activation VALUES (1, true, NOW()) ON CONFLICT DO NOTHING;

ALTER TABLE payments_activation RENAME TO on_off_switch;


-- TABLE: disbursement (previously known as payments_disbursement)
CREATE TABLE IF NOT EXISTS public.payments_disbursement (
    id character varying(64) NOT NULL PRIMARY KEY,
    requested_at timestamp with time zone NOT NULL
);
CREATE INDEX IF NOT EXISTS payments_di_request_16523d_idx ON public.payments_disbursement USING btree (requested_at DESC);
ALTER INDEX payments_di_request_16523d_idx RENAME TO disbursement_request_16523d_idx;

ALTER TABLE payments_disbursement RENAME TO disbursements;


-- TABLE: payments_semaphore (previously known as payments_heartbeat)
CREATE TABLE IF NOT EXISTS public.payments_heartbeat (
    id BIGSERIAL PRIMARY KEY,
    name character varying(128) NOT NULL,
    last_beat timestamp with time zone NOT NULL
);
ALTER TABLE payments_heartbeat RENAME TO payments_semaphore;


-- TABLE: payment (previously known as payments_payment)
CREATE TABLE IF NOT EXISTS public.payments_payment (
    id character varying(64) NOT NULL PRIMARY KEY,
    stellar_transaction_id character varying(64),
    custodial_payment_id text,
    status character varying(32) NOT NULL,
    status_message text,
    requested_at timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    account_id character varying(64) NOT NULL REFERENCES public.receivers(id) DEFERRABLE INITIALLY DEFERRED,
    disbursement_id character varying(64) NOT NULL REFERENCES public.disbursements(id) DEFERRABLE INITIALLY DEFERRED,
    amount numeric(7,2) NOT NULL,
    idempotency_key character varying(64) NOT NULL,
    withdrawal_amount numeric(7,2) NOT NULL,
    withdrawal_status character varying(32) NOT NULL
);
CREATE INDEX IF NOT EXISTS payments_pa_request_4ce797_idx ON public.payments_payment USING btree (requested_at DESC);
ALTER INDEX payments_pa_request_4ce797_idx RENAME TO payment_requested_at_idx;

CREATE INDEX IF NOT EXISTS payments_payment_account_id_af225a32 ON public.payments_payment USING btree (account_id);
ALTER INDEX payments_payment_account_id_af225a32 RENAME TO payment_account_id_idx;

CREATE INDEX IF NOT EXISTS payments_payment_account_id_af225a32_like ON public.payments_payment USING btree (account_id varchar_pattern_ops);
ALTER INDEX payments_payment_account_id_af225a32_like RENAME TO payment_account_id_like_idx;

CREATE INDEX IF NOT EXISTS payments_payment_disbursement_id_2a817b83 ON public.payments_payment USING btree (disbursement_id);
ALTER INDEX payments_payment_disbursement_id_2a817b83 RENAME TO payment_disbursement_id_idx;

ALTER TABLE payments_payment RENAME TO payments;


-- TABLE: payments_withdrawal
CREATE TABLE IF NOT EXISTS public.payments_withdrawal (
    sep24_transaction_id character varying(64) NOT NULL PRIMARY KEY,
    anchor_id character varying(64) NOT NULL,
    amount numeric(7,2) NOT NULL,
    started_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    account_id character varying(64) NOT NULL REFERENCES public.receivers (id) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX IF NOT EXISTS payments_wi_created_18b04a_idx ON public.payments_withdrawal USING btree (created_at DESC);
ALTER INDEX payments_wi_created_18b04a_idx RENAME TO withdrawal_created_at_idx;

CREATE INDEX IF NOT EXISTS payments_withdrawal_account_id_ec0819dd ON public.payments_withdrawal USING btree (account_id);
ALTER INDEX payments_withdrawal_account_id_ec0819dd RENAME TO withdrawal_account_id_idx;

CREATE INDEX IF NOT EXISTS payments_withdrawal_account_id_ec0819dd_like ON public.payments_withdrawal USING btree (account_id varchar_pattern_ops);
ALTER INDEX payments_withdrawal_account_id_ec0819dd_like RENAME TO withdrawal_account_id_like_idx;

ALTER TABLE payments_withdrawal RENAME TO withdrawal;


-- +migrate Down

DROP TABLE IF EXISTS public.withdrawal CASCADE;             -- Called 'payments_withdrawal' in SDP-v1
DROP TABLE IF EXISTS public.payments CASCADE;               -- Called 'payments_payment' in SDP-v1
DROP TABLE IF EXISTS public.payments_semaphore CASCADE;     -- Called 'payments_heartbeat' in SDP-v1
DROP TABLE IF EXISTS public.disbursements CASCADE;           -- Called 'payments_disbursement' in SDP-v1
DROP TABLE IF EXISTS public.on_off_switch CASCADE;          -- Called 'payments_activation' in SDP-v1
DROP TABLE IF EXISTS public.receivers CASCADE;               -- Called 'payments_account' in SDP-v1

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
