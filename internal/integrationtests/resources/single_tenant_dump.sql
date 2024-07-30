--
-- PostgreSQL database dump
--

-- Dumped from database version 12.17
-- Dumped by pg_dump version 12.19 (Debian 12.19-1.pgdg120+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: disbursement_status; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.disbursement_status AS ENUM (
    'DRAFT',
    'READY',
    'STARTED',
    'PAUSED',
    'COMPLETED'
);


ALTER TYPE public.disbursement_status OWNER TO postgres;

--
-- Name: message_status; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.message_status AS ENUM (
    'PENDING',
    'SUCCESS',
    'FAILURE'
);


ALTER TYPE public.message_status OWNER TO postgres;

--
-- Name: message_type; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.message_type AS ENUM (
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL',
    'DRY_RUN'
);


ALTER TYPE public.message_type OWNER TO postgres;

--
-- Name: payment_status; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.payment_status AS ENUM (
    'DRAFT',
    'READY',
    'PENDING',
    'PAUSED',
    'SUCCESS',
    'FAILED',
    'CANCELED'
);


ALTER TYPE public.payment_status OWNER TO postgres;

--
-- Name: receiver_wallet_status; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.receiver_wallet_status AS ENUM (
    'DRAFT',
    'READY',
    'REGISTERED',
    'FLAGGED'
);


ALTER TYPE public.receiver_wallet_status OWNER TO postgres;

--
-- Name: transaction_status; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.transaction_status AS ENUM (
    'PENDING',
    'PROCESSING',
    'SUCCESS',
    'ERROR'
);


ALTER TYPE public.transaction_status OWNER TO postgres;

--
-- Name: verification_type; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.verification_type AS ENUM (
    'DATE_OF_BIRTH',
    'PIN',
    'NATIONAL_ID_NUMBER'
);


ALTER TYPE public.verification_type OWNER TO postgres;

--
-- Name: auth_user_mfa_codes_before_update(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.auth_user_mfa_codes_before_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.auth_user_mfa_codes_before_update() OWNER TO postgres;

--
-- Name: auth_user_password_reset_before_insert(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.auth_user_password_reset_before_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE
        auth_user_password_reset
    SET
        is_valid = false
    WHERE
        auth_user_id = NEW.auth_user_id;

    RETURN NEW;
END;
$$;


ALTER FUNCTION public.auth_user_password_reset_before_insert() OWNER TO postgres;

--
-- Name: create_disbursement_status_history(timestamp with time zone, public.disbursement_status, character varying); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.create_disbursement_status_history(time_stamp timestamp with time zone, disb_status public.disbursement_status, user_id character varying) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', disb_status,
            'user_id', user_id
        );
	END;
$$;


ALTER FUNCTION public.create_disbursement_status_history(time_stamp timestamp with time zone, disb_status public.disbursement_status, user_id character varying) OWNER TO postgres;

--
-- Name: create_message_status_history(timestamp with time zone, public.message_status, character varying); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.create_message_status_history(time_stamp timestamp with time zone, m_status public.message_status, status_message character varying) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
	BEGIN
        RETURN jsonb_build_object(
            'timestamp', time_stamp,
            'status', m_status,
            'status_message', status_message
        );
	END;
$$;


ALTER FUNCTION public.create_message_status_history(time_stamp timestamp with time zone, m_status public.message_status, status_message character varying) OWNER TO postgres;

--
-- Name: create_payment_status_history(timestamp with time zone, public.payment_status, character varying); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.create_payment_status_history(time_stamp timestamp with time zone, pay_status public.payment_status, status_message character varying) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
	BEGIN
        RETURN json_build_object(
            'timestamp', time_stamp,
            'status', pay_status,
            'status_message', status_message
        );
	END;
$$;


ALTER FUNCTION public.create_payment_status_history(time_stamp timestamp with time zone, pay_status public.payment_status, status_message character varying) OWNER TO postgres;

--
-- Name: create_receiver_wallet_status_history(timestamp with time zone, public.receiver_wallet_status); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.create_receiver_wallet_status_history(time_stamp timestamp with time zone, rw_status public.receiver_wallet_status) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', rw_status
        );
	END;
$$;


ALTER FUNCTION public.create_receiver_wallet_status_history(time_stamp timestamp with time zone, rw_status public.receiver_wallet_status) OWNER TO postgres;

--
-- Name: create_submitter_transactions_status_history(timestamp with time zone, public.transaction_status, character varying, text, text, text); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.create_submitter_transactions_status_history(time_stamp timestamp with time zone, tss_status public.transaction_status, status_message character varying, stellar_transaction_hash text, xdr_sent text, xdr_received text) RETURNS jsonb
    LANGUAGE plpgsql
    AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', tss_status,
            'status_message', status_message,
            'stellar_transaction_hash', stellar_transaction_hash,
            'xdr_sent', xdr_sent,
            'xdr_received', xdr_received
        );
	END;
$$;


ALTER FUNCTION public.create_submitter_transactions_status_history(time_stamp timestamp with time zone, tss_status public.transaction_status, status_message character varying, stellar_transaction_hash text, xdr_sent text, xdr_received text) OWNER TO postgres;

--
-- Name: enforce_single_row_for_organizations(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.enforce_single_row_for_organizations() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF (SELECT COUNT(*) FROM public.organizations) != 0 THEN
    RAISE EXCEPTION 'public.organizations can must contain exactly one row';
  END IF;
  RETURN NEW;
END;
$$;


ALTER FUNCTION public.enforce_single_row_for_organizations() OWNER TO postgres;

--
-- Name: update_at_refresh(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.update_at_refresh() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;   
END;
$$;


ALTER FUNCTION public.update_at_refresh() OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: assets; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.assets (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    code character varying(12) NOT NULL,
    issuer character varying(56) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT asset_issuer_length_check CHECK (((((code)::text = 'XLM'::text) AND (char_length((issuer)::text) = 0)) OR (char_length((issuer)::text) = 56)))
);


ALTER TABLE public.assets OWNER TO postgres;

--
-- Name: auth_migrations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.auth_migrations (
    id text NOT NULL,
    applied_at timestamp with time zone
);


ALTER TABLE public.auth_migrations OWNER TO postgres;

--
-- Name: auth_user_mfa_codes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.auth_user_mfa_codes (
    device_id text NOT NULL,
    auth_user_id character varying(36) NOT NULL,
    code character varying(8),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    device_expires_at timestamp with time zone,
    code_expires_at timestamp with time zone
);


ALTER TABLE public.auth_user_mfa_codes OWNER TO postgres;

--
-- Name: auth_user_password_reset; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.auth_user_password_reset (
    token text NOT NULL,
    auth_user_id character varying(36) NOT NULL,
    is_valid boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.auth_user_password_reset OWNER TO postgres;

--
-- Name: auth_users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.auth_users (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    encrypted_password text NOT NULL,
    email text NOT NULL,
    is_owner boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    roles text[],
    is_active boolean DEFAULT true,
    first_name character varying(128) DEFAULT ''::character varying NOT NULL,
    last_name character varying(128) DEFAULT ''::character varying NOT NULL
);


ALTER TABLE public.auth_users OWNER TO postgres;

--
-- Name: channel_accounts; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.channel_accounts (
    public_key character varying(64) NOT NULL,
    private_key character varying(256),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    locked_at timestamp with time zone,
    locked_until_ledger_number integer
);


ALTER TABLE public.channel_accounts OWNER TO postgres;

--
-- Name: countries; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.countries (
    code character varying(3) NOT NULL,
    name character varying(100) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT country_code_length_check CHECK ((char_length((code)::text) = 3))
);


ALTER TABLE public.countries OWNER TO postgres;

--
-- Name: disbursements; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.disbursements (
    id character varying(64) DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    asset_id character varying(36) NOT NULL,
    country_code character varying(3) NOT NULL,
    wallet_id character varying(36) NOT NULL,
    name character varying(128) NOT NULL,
    status public.disbursement_status DEFAULT 'DRAFT'::public.disbursement_status NOT NULL,
    status_history jsonb[] DEFAULT ARRAY[public.create_disbursement_status_history(now(), 'DRAFT'::public.disbursement_status, NULL::character varying)] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    verification_field public.verification_type DEFAULT 'DATE_OF_BIRTH'::public.verification_type NOT NULL,
    file_content bytea,
    file_name text,
    sms_registration_message_template text
);


ALTER TABLE public.disbursements OWNER TO postgres;

--
-- Name: gorp_migrations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.gorp_migrations (
    id text NOT NULL,
    applied_at timestamp with time zone
);


ALTER TABLE public.gorp_migrations OWNER TO postgres;

--
-- Name: messages; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.messages (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    type public.message_type NOT NULL,
    asset_id character varying(36),
    wallet_id character varying(36) NOT NULL,
    receiver_id character varying(36) NOT NULL,
    text_encrypted character varying(1024) NOT NULL,
    title_encrypted character varying(128),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    status public.message_status DEFAULT 'PENDING'::public.message_status NOT NULL,
    status_history jsonb[] DEFAULT ARRAY[public.create_message_status_history(now(), 'PENDING'::public.message_status, NULL::character varying)] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    receiver_wallet_id character varying(36)
);


ALTER TABLE public.messages OWNER TO postgres;

--
-- Name: organizations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.organizations (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(64) NOT NULL,
    timezone_utc_offset character varying(6) DEFAULT '+00:00'::character varying NOT NULL,
    sms_registration_message_template character varying(255) DEFAULT 'You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register.'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    logo bytea,
    is_approval_required boolean DEFAULT false NOT NULL,
    otp_message_template character varying(255) DEFAULT '{{.OTP}} is your {{.OrganizationName}} phone verification code.'::character varying NOT NULL,
    sms_resend_interval integer,
    payment_cancellation_period_days integer,
    CONSTRAINT organization_name_not_empty_check CHECK ((char_length((name)::text) > 1)),
    CONSTRAINT organization_sms_resend_interval_valid_value_check CHECK ((((sms_resend_interval IS NOT NULL) AND (sms_resend_interval > 0)) OR (sms_resend_interval IS NULL))),
    CONSTRAINT organization_timezone_size_check CHECK ((char_length((timezone_utc_offset)::text) = 6))
);


ALTER TABLE public.organizations OWNER TO postgres;

--
-- Name: COLUMN organizations.is_approval_required; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.organizations.is_approval_required IS 'Column used to enable disbursement approval for organizations, requiring multiple users to start a disbursement.';


--
-- Name: payments; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.payments (
    id character varying(64) DEFAULT public.uuid_generate_v4() NOT NULL,
    stellar_transaction_id character varying(64),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    receiver_id character varying(64) NOT NULL,
    disbursement_id character varying(64) NOT NULL,
    amount numeric(19,7) NOT NULL,
    asset_id character varying(36) NOT NULL,
    stellar_operation_id character varying(32),
    blockchain_sender_id character varying(69),
    status public.payment_status DEFAULT 'DRAFT'::public.payment_status NOT NULL,
    status_history jsonb[] DEFAULT ARRAY[public.create_payment_status_history(now(), 'DRAFT'::public.payment_status, NULL::character varying)] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    receiver_wallet_id character varying(64) NOT NULL,
    external_payment_id character varying(64)
);


ALTER TABLE public.payments OWNER TO postgres;

--
-- Name: receiver_verifications; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.receiver_verifications (
    receiver_id character varying(64) NOT NULL,
    verification_field public.verification_type NOT NULL,
    hashed_value text NOT NULL,
    attempts smallint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    confirmed_at timestamp with time zone,
    failed_at timestamp with time zone
);


ALTER TABLE public.receiver_verifications OWNER TO postgres;

--
-- Name: receiver_wallets; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.receiver_wallets (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    receiver_id character varying(36) NOT NULL,
    wallet_id character varying(36) NOT NULL,
    stellar_address character varying(56),
    stellar_memo character varying(56),
    stellar_memo_type character varying(56),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    status public.receiver_wallet_status DEFAULT 'DRAFT'::public.receiver_wallet_status NOT NULL,
    status_history jsonb[] DEFAULT ARRAY[public.create_receiver_wallet_status_history(now(), 'DRAFT'::public.receiver_wallet_status)] NOT NULL,
    otp text,
    otp_created_at timestamp with time zone,
    otp_confirmed_at timestamp with time zone,
    anchor_platform_transaction_id text,
    invitation_sent_at timestamp with time zone,
    anchor_platform_transaction_synced_at timestamp with time zone
);


ALTER TABLE public.receiver_wallets OWNER TO postgres;

--
-- Name: receivers; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.receivers (
    id character varying(64) DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    phone_number character varying(32) NOT NULL,
    email character varying(254),
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    external_id character varying(64)
);


ALTER TABLE public.receivers OWNER TO postgres;

--
-- Name: submitter_transactions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.submitter_transactions (
    id character varying(64) DEFAULT public.uuid_generate_v4() NOT NULL,
    external_id character varying(64) NOT NULL,
    status_message text,
    asset_code character varying(12) NOT NULL,
    asset_issuer character varying(56) NOT NULL,
    amount numeric(19,7) NOT NULL,
    destination character varying(56) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    sent_at timestamp with time zone,
    completed_at timestamp with time zone,
    stellar_transaction_hash character varying(64),
    attempts_count integer DEFAULT 0,
    synced_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    xdr_sent text,
    xdr_received text,
    locked_at timestamp with time zone,
    locked_until_ledger_number integer,
    status public.transaction_status DEFAULT 'PENDING'::public.transaction_status NOT NULL,
    status_history jsonb[] DEFAULT ARRAY[public.create_submitter_transactions_status_history(now(), 'PENDING'::public.transaction_status, NULL::character varying, NULL::text, NULL::text, NULL::text)],
    CONSTRAINT asset_issuer_length_check CHECK (((((asset_code)::text = 'XLM'::text) AND (char_length((asset_issuer)::text) = 0)) OR (char_length((asset_issuer)::text) = 56))),
    CONSTRAINT check_retry_count CHECK ((attempts_count >= 0))
);


ALTER TABLE public.submitter_transactions OWNER TO postgres;

--
-- Name: wallets; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.wallets (
    id character varying(36) DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(30) NOT NULL,
    homepage character varying(255) NOT NULL,
    deep_link_schema character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    sep_10_client_domain character varying(255) DEFAULT ''::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL
);


ALTER TABLE public.wallets OWNER TO postgres;

--
-- Name: wallets_assets; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.wallets_assets (
    wallet_id character varying(36),
    asset_id character varying(36)
);


ALTER TABLE public.wallets_assets OWNER TO postgres;

--
-- Data for Name: assets; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.assets (id, code, issuer, created_at, updated_at, deleted_at) FROM stdin;
4c62168d-b092-4073-b1c2-0e4c19377188	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	2023-06-02 17:26:12.256765+00	2023-06-02 17:26:12.256765+00	\N
8cf40625-7eb8-49e6-bd29-0352a175d059	EUROC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	2023-06-02 17:26:12.45239+00	2023-06-02 17:26:12.45239+00	\N
e7cc851e-ed85-479f-a68d-8c74cadfa755	XLM		2023-09-06 00:55:30.430546+00	2023-09-06 00:55:30.430546+00	\N
\.


--
-- Data for Name: auth_migrations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.auth_migrations (id, applied_at) FROM stdin;
2023-02-09.0.add-users-table.sql	2023-06-02 17:26:12.55839+00
2023-03-07.0.add-password-reset-table.sql	2023-06-02 17:26:12.569327+00
2023-03-10.0.alter-users-table-add-roles-column.sql	2023-06-02 17:26:12.571269+00
2023-03-22.0.alter-users-table-add-is_active-column.sql	2023-06-02 17:26:12.573017+00
2023-03-28.0.alter-users-table-add-new-columns-and-drop-username-column.sql	2023-06-02 17:26:12.575227+00
2023-07-20.0-create-auth_user_mfa_codes_table.sql	2023-09-06 00:48:57.3333+00
\.


--
-- Data for Name: auth_user_mfa_codes; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.auth_user_mfa_codes (device_id, auth_user_id, code, created_at, updated_at, device_expires_at, code_expires_at) FROM stdin;
df6ffbf0-f5ab-450c-a906-4a6841d52eac	1888f2c6-7ffd-4af7-833a-3ee34e6603db	\N	2023-09-28 16:55:44.521235+00	2024-03-20 18:53:04.080603+00	2024-03-27 18:53:04.061926+00	\N
9e86495b-eb10-41c6-a70a-c450b7bdb4cf	e6d26f49-7736-4cea-bb4f-0f3cb573fae5	\N	2023-10-17 17:49:06.278766+00	2024-06-10 14:59:24.701781+00	2024-06-17 14:59:24.677946+00	\N
0f20c606-a1eb-4952-afec-dcdca83ff897	32097fa4-c3fb-4829-a509-c1e95c75ae62	\N	2024-04-17 21:33:09.948818+00	2024-04-17 21:33:29.236204+00	2024-04-24 21:33:29.213983+00	\N
6b6d0712-caab-49e8-ba5e-4a5dcf0b0103	eba6665d-aeb9-4918-a9ea-07d9c52fe23c	002147	2023-09-19 20:16:12.22511+00	2023-09-19 20:23:15.325055+00	\N	2023-09-19 20:28:15.326416+00
9e86495b-eb10-41c6-a70a-c450b7bdb4cf	b4e1354e-6998-4910-814c-8bd21d8e70ba	\N	2024-02-27 20:39:54.168166+00	2024-02-27 20:40:08.912322+00	2024-03-05 20:40:08.88545+00	\N
4c798efb-6908-421b-a847-ae0e2f28d00e	e2c2d239-a4e6-4a8d-98d0-372708b7769e	\N	2024-04-25 19:33:55.158231+00	2024-04-25 19:34:21.315844+00	2024-05-02 19:34:21.288711+00	\N
9e86495b-eb10-41c6-a70a-c450b7bdb4cf	fec3508c-8414-42e8-9e9b-d1144d135991	536456	2023-10-17 17:48:39.778657+00	2023-10-23 22:57:54.304632+00	\N	2023-10-23 23:02:54.300583+00
d41411eb-f78b-403d-8726-24a23833439b	fec3508c-8414-42e8-9e9b-d1144d135991	493318	2023-10-30 17:29:31.996695+00	2023-10-30 17:29:31.996695+00	\N	2023-10-30 17:34:32.000395+00
d41411eb-f78b-403d-8726-24a23833439b	71807142-7483-4c75-b1b4-a382323fcd0f	664640	2023-10-30 17:30:30.197115+00	2023-10-30 17:30:30.197115+00	\N	2023-10-30 17:35:30.200797+00
f8b8898b-24dd-40d3-965b-3802b29cbe2a	42fcd8be-cc7f-4087-bd54-1716fe26fa12	\N	2024-04-30 06:36:44.164286+00	2024-04-30 06:37:07.105476+00	2024-05-07 06:37:07.077717+00	\N
d41411eb-f78b-403d-8726-24a23833439b	c7032759-cf6a-4494-bce5-014961f77820	\N	2023-10-30 17:34:25.997118+00	2023-10-30 17:34:51.139664+00	2023-11-06 17:34:51.119285+00	\N
9bda184f-378f-4b8d-90f9-0fa520cf6368	66c98f75-4368-4913-8757-b8d41225c609	\N	2024-02-23 21:15:44.244549+00	2024-05-01 13:37:26.003546+00	2024-05-08 13:37:25.985541+00	\N
9bda184f-378f-4b8d-90f9-0fa520cf6368	32097fa4-c3fb-4829-a509-c1e95c75ae62	\N	2024-02-28 20:15:17.96726+00	2024-02-28 20:15:38.644909+00	2024-03-06 20:15:38.616111+00	\N
c91a8dfe-0491-4f9f-8ee8-6373c065d402	66c98f75-4368-4913-8757-b8d41225c609	\N	2023-09-06 01:14:40.106699+00	2024-02-09 19:15:34.851485+00	2024-02-16 19:15:34.830646+00	\N
7b0635fc-1252-4ab9-ab13-8eefba5257ce	f02906dc-feb0-48c4-8376-f26648bae486	\N	2023-10-16 22:47:53.0262+00	2024-07-17 22:58:15.606135+00	2024-07-24 22:58:15.575103+00	\N
ababf015-b8d0-454d-a072-70869a50b799	66c98f75-4368-4913-8757-b8d41225c609	\N	2023-09-06 01:19:13.626213+00	2024-07-17 22:59:30.648542+00	2024-07-24 22:59:30.620179+00	\N
ababf015-b8d0-454d-a072-70869a50b799	32097fa4-c3fb-4829-a509-c1e95c75ae62	\N	2024-02-06 02:45:35.886778+00	2024-02-29 02:23:24.662823+00	2024-03-07 02:23:24.633368+00	\N
4e43a551-1785-47ec-9b85-1f66bf07a9b5	13c0af15-269e-4f13-94a2-ba30e64d1981	\N	2024-02-23 22:20:19.267219+00	2024-03-11 16:27:00.160158+00	2024-03-18 16:27:00.137488+00	\N
\.


--
-- Data for Name: auth_user_password_reset; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.auth_user_password_reset (token, auth_user_id, is_valid, created_at) FROM stdin;
Y1Cmp8atgh	e6d26f49-7736-4cea-bb4f-0f3cb573fae5	f	2023-07-05 22:49:34.742438+00
LfdfaGtzh0	7c19c62a-f05a-4676-8f2e-0bc66e6f08b7	t	2023-07-17 21:45:48.697907+00
kzrBobEjTm	0f129e4c-6a4d-433c-a922-f8203203f348	t	2023-07-17 21:55:15.821328+00
8JD9BLKrd4	73078b8e-4617-476c-8e34-c25e06f2677e	f	2023-07-17 21:48:17.720417+00
PXp8q8HU96	73078b8e-4617-476c-8e34-c25e06f2677e	f	2023-07-17 21:55:56.886549+00
k6WOiHSu6D	73078b8e-4617-476c-8e34-c25e06f2677e	f	2023-07-17 21:56:07.746534+00
fAtpl2Xy4J	86e3b2ad-af15-419c-b7ac-d33c35198413	f	2023-07-17 21:57:27.159719+00
onVJwgMeI5	53510b69-aa3b-4332-a18b-60ad253991f5	f	2023-07-17 21:57:34.231307+00
wvN0dbh46k	7e930e21-a5ee-4e09-9bbe-acaabcc673c3	f	2023-08-17 17:41:52.020442+00
l2Qa20lDf0	1b146511-63fc-4286-b828-8403cc3b83c2	f	2023-08-17 19:36:16.2278+00
8RSpRlFZ8Z	f02906dc-feb0-48c4-8376-f26648bae486	f	2023-09-19 22:45:05.896337+00
EblWd1BRhb	f02906dc-feb0-48c4-8376-f26648bae486	f	2023-10-16 22:47:09.472406+00
yXceAQVI6X	c7032759-cf6a-4494-bce5-014961f77820	f	2023-07-17 16:37:21.205316+00
F8xwq4KnAb	c7032759-cf6a-4494-bce5-014961f77820	f	2023-07-17 17:21:03.390257+00
zfNUfJOvKh	c7032759-cf6a-4494-bce5-014961f77820	f	2023-07-17 17:50:06.991901+00
SJ7Abid6Yz	c7032759-cf6a-4494-bce5-014961f77820	f	2023-07-17 17:52:58.168792+00
Rdz1Ds4ATi	c7032759-cf6a-4494-bce5-014961f77820	f	2023-07-28 18:35:31.862075+00
XSKtz3sbQC	c7032759-cf6a-4494-bce5-014961f77820	f	2023-10-30 17:32:24.696499+00
hhu7TQ0Asl	32097fa4-c3fb-4829-a509-c1e95c75ae62	f	2024-02-06 02:39:44.897673+00
3yAGyXP3I8	32097fa4-c3fb-4829-a509-c1e95c75ae62	f	2024-02-06 02:44:30.264815+00
xr0mRtKNYI	13c0af15-269e-4f13-94a2-ba30e64d1981	f	2024-02-23 22:19:44.316539+00
9kk7Q5lT5r	b4e1354e-6998-4910-814c-8bd21d8e70ba	f	2024-02-27 20:36:13.236823+00
NkaDxetHBE	1888f2c6-7ffd-4af7-833a-3ee34e6603db	f	2023-08-17 12:21:37.454496+00
qfWflJCg6t	1888f2c6-7ffd-4af7-833a-3ee34e6603db	f	2023-09-28 16:54:49.712146+00
rlxF9eoW8b	1888f2c6-7ffd-4af7-833a-3ee34e6603db	f	2024-03-20 18:52:00.515443+00
YHP8aGIKu7	e2c2d239-a4e6-4a8d-98d0-372708b7769e	f	2024-04-25 19:32:31.297648+00
qFm1mQweon	42fcd8be-cc7f-4087-bd54-1716fe26fa12	f	2024-04-30 06:34:52.126436+00
\.


--
-- Data for Name: auth_users; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.auth_users (id, encrypted_password, email, is_owner, created_at, roles, is_active, first_name, last_name) FROM stdin;
eba6665d-aeb9-4918-a9ea-07d9c52fe23c	$2a$10$d/OrXRFJQoYb528yA61jX.b8h26JuCKaK1Ngf0E30H.ScYiKLlkrG	jake+owner@stellar.org	t	2023-06-06 20:30:51.89708+00	{owner}	t	Jake	Owner
d4ffa91c-be6b-4a5a-88a3-21b5d8410559	$2a$10$99c1YTs63qlZfDjNb5IFQ.ffyNcL3Gmex3pKe79PRTSqSoqtZcli2	nando+owner@stellar.org	t	2023-06-12 20:36:19.00376+00	{owner}	t	Nando	Owner
fec3508c-8414-42e8-9e9b-d1144d135991	$2a$10$MVSZvGFptylxh76n8syJzuXGoepf47dvJGs4qKRfL5o4NBk6Oyz1G	audit.owner@stellar.org	f	2023-07-14 20:22:36.481631+00	{owner}	t	owner	auditor
71807142-7483-4c75-b1b4-a382323fcd0f	$2a$10$CqfVoNSm5DKYvbKsjM5zi.qJD.MuzVOW9NTsN.S9pku7qit/jsWOW	audit.financial@stellar.org	f	2023-07-14 20:28:52.488267+00	{financial_controller}	t	financial	auditor
6a12d770-aa39-4372-b944-37380be181a8	$2a$10$wKGH532/IwxEwDs9Kasa6eGuADRoChmpq9axtCIGMi9UIhI58q4IW	audit.developer@stellar.org	f	2023-07-14 20:29:37.872054+00	{developer}	t	developer	auditor
7d609e78-29be-41bd-b86b-4f4105b5d688	$2a$10$xFaRPtrzKynTZ1pb8V/pp.hn6oR4nzSb3gSQrPClrStSImwLL0n42	audit.business@stellar.org	f	2023-07-14 20:30:11.183525+00	{business}	t	business	auditor
644aef15-41ad-4f21-a9f3-ae0356a88d69	$2a$10$DSUn7R0VMJ1mldxi0YAMmeim9enL4qVDsJAxuuOoYfdrJiDbQlOye	msequeira+t1@coinspect.com	f	2023-07-17 22:16:51.553748+00	{financial_controller}	f	test1	test1
3321a56b-01ba-42f2-b250-82245a2d31b4	$2a$10$qraJpicxBarGkDolA0vTme0FS9Wn4QnYx62HNCZN9FqJZR3vhKvhi	msequeira+t3@coinspect.com	f	2023-07-17 22:38:01.057056+00	{financial_controller}	t	test3	test3
7c19c62a-f05a-4676-8f2e-0bc66e6f08b7	$2a$10$rDetABe/ZxN4wv2kv04JKuBPAWmWZlKCWfHdNqqd1JgDrqmDFNqR.	msequeira.business@coinspect.com	f	2023-07-17 21:35:46.85659+00	{business}	f	MatiasBusiness	Coinspect
0f129e4c-6a4d-433c-a922-f8203203f348	$2a$10$Y/KbGJT5nVAl4IBuHF8YHuHPvM4AWKQfhhnNLAYwJc67tsQh4rD1.	msequeira.developer@coinspect.com	f	2023-07-17 21:35:01.252207+00	{developer}	f	MatiasDeveloper	Coinspect
c412fbbe-e7dc-4ff7-be79-a5119db41b8a	$2a$10$xb16snCtVZKqHF9eHttfAOpkWHy2UL2P0.j1gRjNBaRWxeV56VXv6	msequeira.financialcont@coinspect.com	f	2023-07-17 21:33:57.958158+00	{financial_controller}	f	MatiasFinancialCont	Coinspect
53510b69-aa3b-4332-a18b-60ad253991f5	$2a$10$vzuF2/1f.Txae5EnZK3hN.Y6xgzl/CrVgjlzPrzB1X3Z4W9Muwaze	msequeira+financial@coinspect.com	f	2023-07-17 21:55:05.643189+00	{financial_controller}	t	MatiasFinanciall	Coinspect
73078b8e-4617-476c-8e34-c25e06f2677e	$2a$10$E195lifXN3N8BzYt5raSoO1ALbXsLjC7WCVnjk0P/txEZzrwr4ihi	msequeira+business@coinspect.com	f	2023-07-17 21:47:53.360226+00	{business}	t	MatiasBusiness2	Coinspect
86e3b2ad-af15-419c-b7ac-d33c35198413	$2a$10$PwIrwDTIRZ/xGydsTe7kOel1fmwlDnqrULS2RZJlmLoBUtiYE6EH2	msequeira+developer@coinspect.com	f	2023-07-17 21:54:35.860308+00	{developer}	t	MatiasDeveloper	Coinspect
64bafd6b-2252-494e-b50e-3791cf1f3201	$2a$10$y6tFuqBAMSI6uXVk5iW6MeX9kiYZaFFPpMhQtglRITN8JFivjkww6	natam.oliveira+owner@ckl.io	f	2023-08-16 22:39:04.374692+00	{owner}	t	Natam	Owner
240b9cfa-c566-474f-be5c-0148b23dcf06	$2a$10$kyJ2JsPBhlobVAR7t9E81eZyzOp1MAihyfGB4DoLNAuFn/A8Z.6ua	msequeira+t2@coinspect.com	f	2023-07-17 22:31:18.658787+00	{business}	t	test2	test2
928b0f3e-a61c-4be0-9420-c01a015cda7c	$2a$10$o61TUDv1e1iN9Wa61JcI7eJlA.BKIEzRYmIeDX4HrKXYbHdHYyFSq	erica.liu+owner@stellar.org	t	2023-09-19 23:10:28.924608+00	{owner}	t	Erica	Owner
7ad32e05-bea3-4472-bf9e-33f562cb0069	$2a$10$yWZeJNaMVufD7zYKSeHDfOghSFKZ4ua4v8lYa7RXafegCiqfRtz9u	msequeira+t23@coinspect.com	f	2023-07-17 22:32:48.546237+00	{developer}	f	test2	test
32097fa4-c3fb-4829-a509-c1e95c75ae62	$2a$10$acBudGvjaqptIJmhN8nI6u7IFFs71/ir15Q93ekbmKnvFnct/h5EO	marcelo+financial@stellar.org	f	2024-02-06 02:38:23.687569+00	{financial_controller}	t	Marcelo	Financial
7e930e21-a5ee-4e09-9bbe-acaabcc673c3	$2a$10$4RUFHDVBssRJcNKTn4X0IuuLIbeQ4mMKrLCQoGRBuUMBUB8116k/K	fabricius+owner@ckl.io	f	2023-08-16 22:39:46.969326+00	{owner}	t	Fabricius	Owner
1b146511-63fc-4286-b828-8403cc3b83c2	$2a$10$15.xcmbi0JAl4u5KzKT.7eu10y1fUMnW68iEV4HxGWWFzuhPOe2iu	gracietti+owner@ckl.io	f	2023-08-16 22:39:19.6733+00	{owner}	t	Gracietti	Owner
f02906dc-feb0-48c4-8376-f26648bae486	$2a$10$H9ydjghbWoLAszSsRjDlMubt.y.tP01aIWOEWnlx/GgW3WU6Rnpjq	reece+owner@stellar.org	t	2023-09-19 21:40:32.008386+00	{owner}	t	Reece	Owner
97c2e8db-72b5-4303-a2c6-e738913832b4	$2a$10$1F4fh0fLvA6rj99r4aDb7eOSjhM1I1LeWbt3Y/SctXvP0XPrGyYDy	marwen.abid+owner@stellar.org	f	2023-10-17 18:27:43.793619+00	{owner}	f	Marwen	Owner
e6d26f49-7736-4cea-bb4f-0f3cb573fae5	$2a$10$QqKHI/2BeEQuyCpNDh/7NOrET1juB63o6YKtw3thqbY/yQ184iW12	marwen.abid@stellar.org	t	2023-07-05 21:04:59.189365+00	{owner}	t	marwen	abid
c7032759-cf6a-4494-bce5-014961f77820	$2a$10$n8YYS9CU6techI3GwFaOrO4JCoj597UDfplY6R/CJMs69aciq/C8q	msequeira@coinspect.com	f	2023-07-17 16:32:42.357297+00	{owner}	t	Matias'"<>	'"<>Coinspect
e2c2d239-a4e6-4a8d-98d0-372708b7769e	$2a$10$3xPQG5.IO.cyf.OyN833Y.TKQDFhqdyiWzeG3JmlzY4v0IStXfQR6	danny.gamboa@stellar.org	f	2024-04-25 13:38:58.657217+00	{owner}	t	Danny	Gamboa
13c0af15-269e-4f13-94a2-ba30e64d1981	$2a$10$fInprKEnPcdTmDMBupatqORBkF2NKYVcwnHxV1pHvczwx5C1YyZ92	tori+owner@stellar.org	t	2023-06-02 17:28:20.073996+00	{owner}	t	Tori	Owner
b4e1354e-6998-4910-814c-8bd21d8e70ba	$2a$10$saXqYTyzCoveSUjDso1q9.85Meh9mzJ7ffcKHZAahDuxeBIJzce7S	marwen.abid+financial@stellar.org	f	2024-02-27 20:35:48.759509+00	{financial_controller}	t	Marwen	Financial
1888f2c6-7ffd-4af7-833a-3ee34e6603db	$2a$10$FTGso42U625OtbtYbCdqGuZexVZAT6QO2zXTT25RPvA4iHreYV7N2	caio.teixeira+owner@ckl.io	f	2023-08-16 22:39:33.671756+00	{owner}	t	Caio	Owner
42fcd8be-cc7f-4087-bd54-1716fe26fa12	$2a$10$vcD0.Kw45Hug4OZ0DAXWIODvTkPk1vOcReYiZXJ0KWpW.7mRczaP6	nick@stellar.org	f	2024-03-11 16:27:19.073456+00	{owner}	t	Nick	Gilbert
66c98f75-4368-4913-8757-b8d41225c609	$2a$10$.8zFEWgoymgtqRNWSvqaJevIc0CedA6rT3lBYJFEKtFIpxWPDHlRG	marcelo+owner@stellar.org	t	2023-06-02 17:28:06.674186+00	{owner}	t	Marcelo	Owner
\.


--
-- Data for Name: channel_accounts; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.channel_accounts (public_key, private_key, created_at, updated_at, locked_at, locked_until_ledger_number) FROM stdin;
GBZ6SYNJ4S44GLULIONUAG4ZEGCTJQJX5Z6PTF26IX5PQT3WZDBLBZYQ	HldI56fQ7A66TzQH8w11DOGM7igRQnROGJpcj7T13E9WhTXw/XBwfna9+XXz0aF/Y5fJJKkzFlPnXRtrIirwTNz4DvWPS5WT80pKxNwJVzqfuE8u	2024-06-12 16:41:17.213934+00	2024-06-25 19:30:18.962652+00	\N	\N
GBL5IFAZLGLFWUTIQSMCBUWHWGQFUYCWABQTTIYDJPEEL6YY4QRKUY2W	I1kj1IEsZorFV1/JWgXrFUXDDk1iw0IleyjaulOyYEVoB/9HmWHFphqAvyjzS9a5Gfk4oh+1SRLs+l8i1UptZhgeQY8f1PMWma1uymDaFdiZWnhX	2024-06-12 16:41:17.213934+00	2024-06-25 19:46:56.868753+00	\N	\N
\.


--
-- Data for Name: countries; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.countries (code, name, created_at, updated_at, deleted_at) FROM stdin;
UKR	Ukraine	2023-06-02 17:26:12.269565+00	2023-06-02 17:26:12.269565+00	\N
BRA	Brazil	2023-06-02 17:26:12.45239+00	2023-06-02 17:26:12.45239+00	\N
USA	United States of America	2023-06-02 17:26:12.45239+00	2023-06-02 17:26:12.45239+00	\N
COL	Colombia	2023-06-02 17:26:12.45239+00	2023-06-02 17:26:12.45239+00	\N
AFG	Afghanistan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ALB	Albania	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DZA	Algeria	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ASM	American Samoa	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
AND	Andorra	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
AGO	Angola	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ATG	Antigua and Barbuda	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ARG	Argentina	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ARM	Armenia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ABW	Aruba	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
AUS	Australia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
AUT	Austria	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
AZE	Azerbaijan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BHS	Bahamas	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BHR	Bahrain	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BGD	Bangladesh	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BRB	Barbados	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BLR	Belarus	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BEL	Belgium	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BLZ	Belize	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BEN	Benin	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BMU	Bermuda	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BTN	Bhutan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BOL	Bolivia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BIH	Bosnia and Herzegovina	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BWA	Botswana	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BRN	Brunei	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BGR	Bulgaria	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BFA	Burkina Faso	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BDI	Burundi	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CPV	Cabo Verde	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KHM	Cambodia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CMR	Cameroon	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CAN	Canada	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CAF	Central African Republic	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TCD	Chad	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CHL	Chile	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CHN	China	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
COM	Comoros (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
COG	Congo (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
COK	Cook Islands (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CRI	Costa Rica	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
HRV	Croatia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CYP	Cyprus	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CZE	Czechia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CIV	Côte d'Ivoire (Ivory Coast)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
COD	Democratic Republic of the Congo	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DNK	Denmark	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DJI	Djibouti	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DMA	Dominica	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DOM	Dominican Republic	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ECU	Ecuador	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
EGY	Egypt	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SLV	El Salvador	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GNQ	Equatorial Guinea	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ERI	Eritrea	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
EST	Estonia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SWZ	Eswatini	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ETH	Ethiopia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
FJI	Fiji	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
FIN	Finland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
FRA	France	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GUF	French Guiana	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PYF	French Polynesia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ATF	French Southern Territories (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GAB	Gabon	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GMB	Gambia (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GEO	Georgia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
DEU	Germany	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GHA	Ghana	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GRC	Greece	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GRL	Greenland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GRD	Grenada	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GUM	Guam	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GTM	Guatemala	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GIN	Guinea	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GNB	Guinea-Bissau	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GUY	Guyana	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
HTI	Haiti	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
HND	Honduras	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
HUN	Hungary	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ISL	Iceland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
IND	India	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
IDN	Indonesia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
IRQ	Iraq	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
IRL	Ireland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ISR	Israel	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ITA	Italy	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
JAM	Jamaica	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
JPN	Japan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
JOR	Jordan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KAZ	Kazakhstan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KEN	Kenya	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KIR	Kiribati	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KOR	South Korea	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KWT	Kuwait	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KGZ	Kyrgyzstan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LAO	Laos	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LVA	Latvia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LBN	Lebanon	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LSO	Lesotho	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LBR	Liberia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LBY	Libya	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LIE	Liechtenstein	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LTU	Lithuania	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LUX	Luxembourg	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MDG	Madagascar	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MWI	Malawi	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MYS	Malaysia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MDV	Maldives	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MLI	Mali	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MLT	Malta	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MHL	Marshall Islands (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MTQ	Martinique	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MRT	Mauritania	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MUS	Mauritius	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MEX	Mexico	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
FSM	Micronesia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MDA	Moldova	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MCO	Monaco	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MNG	Mongolia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MNE	Montenegro	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MAR	Morocco	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MOZ	Mozambique	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MMR	Myanmar	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NAM	Namibia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NRU	Nauru	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NPL	Nepal	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NLD	Netherlands (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NZL	New Zealand	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NIC	Nicaragua	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NER	Niger	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NGA	Nigeria	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MKD	North Macedonia (Republic of)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
NOR	Norway	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
OMN	Oman	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PAK	Pakistan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PLW	Palau	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PAN	Panama	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PNG	Papua New Guinea	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PRY	Paraguay	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PER	Peru	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PHL	Philippines (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
POL	Poland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PRT	Portugal	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
PRI	Puerto Rico	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
QAT	Qatar	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ROU	Romania	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
RUS	Russia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
RWA	Rwanda	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
REU	Réunion	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
BLM	Saint Barts	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
KNA	Saint Kitts and Nevis	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LCA	Saint Lucia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
MAF	Saint Martin	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VCT	Saint Vincent and the Grenadines	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
WSM	Samoa	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SMR	San Marino	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
STP	Sao Tome and Principe	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SAU	Saudi Arabia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SEN	Senegal	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SRB	Serbia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SYC	Seychelles	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SLE	Sierra Leone	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SGP	Singapore	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SVK	Slovakia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SVN	Slovenia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SLB	Solomon Islands	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SOM	Somalia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ZAF	South Africa	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SSD	South Sudan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ESP	Spain	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
LKA	Sri Lanka	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SDN	Sudan (the)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SUR	Suriname	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
SWE	Sweden	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
CHE	Switzerland	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TWN	Taiwan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TJK	Tajikistan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TZA	Tanzania	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
THA	Thailand	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TLS	Timor-Leste	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TGO	Togo	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TON	Tonga	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TTO	Trinidad and Tobago	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TUN	Tunisia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TUR	Turkey	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TKM	Turkmenistan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TCA	Turks and Caicos Islands	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
TUV	Tuvalu	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
UGA	Uganda	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ARE	United Arab Emirates	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
GBR	United Kingdom	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
UMI	United States Minor Outlying Islands	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
URY	Uruguay	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
UZB	Uzbekistan	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VUT	Vanuatu	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VEN	Venezuela	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VNM	Vietnam	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VGB	Virgin Islands (British)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
VIR	Virgin Islands (U.S.)	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
YEM	Yemen	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ZMB	Zambia	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
ZWE	Zimbabwe	2023-09-06 00:55:10.595308+00	2023-09-06 00:55:10.595308+00	\N
\.


--
-- Data for Name: disbursements; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.disbursements (id, created_at, asset_id, country_code, wallet_id, name, status, status_history, updated_at, verification_field, file_content, file_name, sms_registration_message_template) FROM stdin;
760a8542-c656-42ba-bcb1-397b24c4f744	2024-06-12 16:37:07.77471+00	4c62168d-b092-4073-b1c2-0e4c19377188	USA	7a0c5a0a-33c1-42b9-a27b-d657567c2925	Payroll 2024-05	STARTED	{"{\\"status\\": \\"DRAFT\\", \\"user_id\\": \\"66c98f75-4368-4913-8757-b8d41225c609\\", \\"timestamp\\": \\"2024-06-12T16:37:07.773651305Z\\"}","{\\"status\\": \\"READY\\", \\"user_id\\": \\"66c98f75-4368-4913-8757-b8d41225c609\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\"}","{\\"status\\": \\"STARTED\\", \\"user_id\\": \\"66c98f75-4368-4913-8757-b8d41225c609\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\"}"}	2024-06-12 16:37:08.210896+00	DATE_OF_BIRTH	\\x70686f6e652c69642c616d6f756e742c766572696669636174696f6e0a2b31343135393431383930362c696e7465726e616c2d69642d412c302e312c323030302d30312d30310a2b353534383939363238363335342c696e7465726e616c2d69642d422c302e312c323030302d30312d30310a2b31343135333030303731302c696e7465726e616c2d69642d432c302e312c323030302d30312d30310a	payroll.csv	
f1e46a04-dbf6-49f7-a928-271a19fa1fd5	2024-07-03 15:34:39.426097+00	4c62168d-b092-4073-b1c2-0e4c19377188	HTI	79308ea6-da07-4520-9db4-1b9b390d5d7e	January Payments	STARTED	{"{\\"status\\": \\"DRAFT\\", \\"user_id\\": \\"f02906dc-feb0-48c4-8376-f26648bae486\\", \\"timestamp\\": \\"2024-07-03T15:34:39.426677625Z\\"}","{\\"status\\": \\"READY\\", \\"user_id\\": \\"f02906dc-feb0-48c4-8376-f26648bae486\\", \\"timestamp\\": \\"2024-07-03T15:34:39.618582+00:00\\"}","{\\"status\\": \\"STARTED\\", \\"user_id\\": \\"f02906dc-feb0-48c4-8376-f26648bae486\\", \\"timestamp\\": \\"2024-07-03T15:34:39.819203+00:00\\"}"}	2024-07-03 15:34:39.819203+00	DATE_OF_BIRTH	\\x70686f6e652c69642c616d6f756e742c766572696669636174696f6e0a2b31373738323436353130302c72656563652d69642d30322c3530302c313936382d30392d3235	demo_disbursement.csv	
\.


--
-- Data for Name: gorp_migrations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.gorp_migrations (id, applied_at) FROM stdin;
2023-01-20.0-initial.sql	2023-06-01 23:47:12.220119+00
2023-01-23.0-dump-from-sdp-v1.sql	2023-06-01 23:47:12.444892+00
2023-01-26.0-delete-all-django-stuff.sql	2023-06-01 23:47:12.464515+00
2023-01-26.1-drop-unused-sdp-v1-tables.sql	2023-06-01 23:47:12.475685+00
2023-01-26.2-updated-at-trigger.sql	2023-06-01 23:47:12.48321+00
2023-01-27.0-create-assets-table.sql	2023-06-02 17:26:12.267357+00
2023-01-27.1-create-countries-table.sql	2023-06-02 17:26:12.275933+00
2023-01-27.2-create-wallets-table.sql	2023-06-02 17:26:12.35124+00
2023-01-27.3-create-receiver-wallets-table.sql	2023-06-02 17:26:12.360136+00
2023-01-27.4-create-messages-table.sql	2023-06-02 17:26:12.368177+00
2023-01-30.0-update-disbursements-table.sql	2023-06-02 17:26:12.386507+00
2023-01-30.1-update-payments-table.sql	2023-06-02 17:26:12.410142+00
2023-01-30.2-drop-unused-payments-columns.sql	2023-06-02 17:26:12.416328+00
2023-01-30.3-update-receivers-table.sql	2023-06-02 17:26:12.41957+00
2023-01-30.4-receiver-wallets-status.sql	2023-06-02 17:26:12.435509+00
2023-02-03.0-update-messages-add-new-columns.sql	2023-06-02 17:26:12.447536+00
2023-03-09.0-populate-static-data-countries-assets-wallets.sql	2023-06-02 17:26:12.453185+00
2023-03-16.0-create-organization-table.sql	2023-06-02 17:26:12.460911+00
2023-03-22.0-enforce-one-row-for-organizations-table.sql	2023-06-02 17:26:12.463362+00
2023-04-12.0-create-submitter-transactions-table.sql	2023-06-02 17:26:12.473848+00
2023-04-17.0-create-receiver_verifications-table.sql	2023-06-02 17:26:12.484687+00
2023-04-21.0-add-receiver-wallets-otp.sql	2023-06-02 17:26:12.486949+00
2023-04-25.0.alter-messages-table-add-receiver-wallet-id.sql	2023-06-02 17:26:12.489388+00
2023-04-26.0-add-demo-wallet.sql	2023-06-02 17:26:12.495413+00
2023-05-01.0-add-sync-column-tss.sql	2023-06-02 17:26:12.496989+00
2023-05-02.0-alter-organizations-table-add-logo.sql	2023-06-02 17:26:12.500638+00
2023-05-23.0-alter-channel-accounts-pk-type.sql	2023-06-02 17:26:12.502175+00
2023-05-31.0-replace-payment-status-enum.sql	2023-06-02 17:26:12.503363+00
2023-06-01.0-add-file-fields-to-disbursements.sql	2023-06-07 19:36:13.750798+00
2023-06-07.0-add-retry-after-column.sql	2023-06-13 18:25:13.803136+00
2023-06-08.0-add-dryrun-message-type.sql	2023-06-13 18:25:13.804819+00
2023-06-22.0-add-unique-constraint-wallet-table.sql	2023-09-06 00:49:23.718587+00
2023-07-05.0-tss-transactions-table-constraints.sql	2023-09-06 00:55:10.533331+00
2023-07-17.0-channel-accounts-management-locks.sql	2023-09-06 00:55:10.540676+00
2023-07-17.1-tss-remove-SENT-status.sql	2023-09-06 00:55:10.54727+00
2023-07-17.2-add-status-history-column-submitter-transactions-table.sql	2023-09-06 00:55:10.56663+00
2023-07-20.0-tss-remove-retry_after-and-rename-retry_count.sql	2023-09-06 00:55:10.587653+00
2023-08-02.0-organizations-table-add-approver-function.sql	2023-09-06 00:55:10.594017+00
2023-08-10.0-countries-seed.sql	2023-09-06 00:55:10.598425+00
2023-08-15.0-alter-issuer-constraints.sql	2023-09-06 00:55:10.60099+00
2023-08-28.0-wallets-countries-and-assets.sql	2023-10-17 18:08:39.698998+00
2023-09-17.0-add-anchor-platform-tx-id.sql	2023-10-17 18:08:39.780243+00
2023-09-20.0-alter-wallets-add-enabled-column.sql	2023-10-17 18:08:39.783422+00
2023-09-21.0-alter-organizations-table-invite-and-otp-messages.sql	2023-10-17 18:08:39.786699+00
2023-09-26.0-fix-payment-status-history-and-remove-unused-org-columns.sql	2023-10-17 18:08:39.791451+00
2023-10-05.0-alter-receiver_wallets-add-invitation_sent_at-column.sql	2023-10-17 18:08:39.796661+00
2023-10-05.1-alter-receiver-wallets-add-anchor-platform-transaction-synced-at.sql	2023-10-17 18:08:39.799666+00
2023-10-12.0.alter-organizations-table-add-sms-resend-interval.sql	2023-10-17 18:08:39.803073+00
2023-10-25.0-update-payments-status-type-and-organizations-table.sql	2024-02-23 21:11:01.04439+00
2023-12-18.0-alter-payments-table-add-external-payment-id.sql	2024-02-23 21:11:01.049252+00
2024-01-12.0-alter-disbursements-table-add-sms-template.sql	2024-02-23 21:11:01.053956+00
2024-02-05.0-tss-transactions-table-amount-constraing.sql	2024-02-23 21:11:01.058698+00
\.


--
-- Data for Name: messages; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.messages (id, type, asset_id, wallet_id, receiver_id, text_encrypted, title_encrypted, created_at, status, status_history, updated_at, receiver_wallet_id) FROM stdin;
02c5ffa3-3dd4-48cb-9005-1d96a0332b53	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	560177da-47a7-4e1c-8361-757fc3ba4c7f	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:05.773325+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:05.773325+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:05.773325+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:05.773325+00	8ec2ff7b-8193-420b-ac40-698dc4d0f139
0558f3af-2f14-4c30-a3af-4f8bbb13d161	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:05.773325+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:05.773325+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:05.773325+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:05.773325+00	c08eea5f-0c25-4611-a869-31bf4844b468
b77c984a-c633-4edc-b48e-46920ad68c61	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	560177da-47a7-4e1c-8361-757fc3ba4c7f	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:15.746255+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:15.746255+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:15.746255+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:15.746255+00	8ec2ff7b-8193-420b-ac40-698dc4d0f139
750bee14-d9d1-45a1-8781-95dab443419d	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:10.767832+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:10.767832+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:10.767832+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:10.767832+00	c08eea5f-0c25-4611-a869-31bf4844b468
9755626e-5130-490e-918e-3c6e9dd452b0	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	560177da-47a7-4e1c-8361-757fc3ba4c7f	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:10.767832+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:10.767832+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:10.767832+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:10.767832+00	8ec2ff7b-8193-420b-ac40-698dc4d0f139
b7ce25eb-9f13-4c62-aa02-bde5da5bd5da	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-25 22:40:15.746255+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-25T22:40:15.746255+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-25T22:40:15.746255+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:40:15.746255+00	c08eea5f-0c25-4611-a869-31bf4844b468
d5b52d13-1801-4588-9be2-d1fa60a4ad27	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	79308ea6-da07-4520-9db4-1b9b390d5d7e	179c2ed5-6dce-46f5-967f-60c4bdfe8f03	You have a payment waiting for you from the SDP Demo Org. Click https://demo-wallet.stellar.org?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b89ae5713e3eda3c41c453b0b10fb73e46a642a4955e248080363c8a7022c6bed979789fe730f58fe128489eb25aab075c091dd5e46536d246511de47a061207 to register.		2024-07-03 15:34:40.68967+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-07-03T15:34:40.68967+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-07-03T15:34:40.68967+00:00\\", \\"status_message\\": null}"}	2024-07-03 15:34:40.68967+00	d8366a73-7fce-44f9-b892-9e80383ba84e
0c479c3f-ac09-40fc-9869-1b044bedc830	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	677820fb-68c4-4595-b0ec-ffa7df5390ef	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-12 16:37:10.908561+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}"}	2024-06-12 16:37:10.908561+00	5a7476b8-df72-4663-b2b3-5031829e04e4
badae3b5-ef36-4d66-a212-43b6861b963a	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-12 16:37:10.908561+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}"}	2024-06-12 16:37:10.908561+00	c08eea5f-0c25-4611-a869-31bf4844b468
df2438cd-d1d1-4abc-94b1-706e7ab639d5	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	7a0c5a0a-33c1-42b9-a27b-d657567c2925	560177da-47a7-4e1c-8361-757fc3ba4c7f	You have a payment waiting for you from the SDP Demo Org. Click https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b99e68b53bc6c26cc0b13f76e51d310ed9b5df406bfc5a3ff1f0b2b3f7b62833698dee3906a722092180b0c65d3f7984c81cc8ea827c7a2113ba9d74e3e36b0f to register.		2024-06-12 16:37:10.908561+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-12T16:37:10.908561+00:00\\", \\"status_message\\": null}"}	2024-06-12 16:37:10.908561+00	8ec2ff7b-8193-420b-ac40-698dc4d0f139
b7b3e38d-9632-45c7-9246-7b7c72ffc5f1	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	79308ea6-da07-4520-9db4-1b9b390d5d7e	179c2ed5-6dce-46f5-967f-60c4bdfe8f03	You have a payment waiting for you from the SDP Demo Org. Click https://demo-wallet.stellar.org?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b89ae5713e3eda3c41c453b0b10fb73e46a642a4955e248080363c8a7022c6bed979789fe730f58fe128489eb25aab075c091dd5e46536d246511de47a061207 to register.		2024-07-05 15:34:45.671916+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-07-05T15:34:45.671916+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-07-05T15:34:45.671916+00:00\\", \\"status_message\\": null}"}	2024-07-05 15:34:45.671916+00	d8366a73-7fce-44f9-b892-9e80383ba84e
37d942b5-cbc6-4df8-a75d-0a32367b3520	TWILIO_SMS	4c62168d-b092-4073-b1c2-0e4c19377188	79308ea6-da07-4520-9db4-1b9b390d5d7e	179c2ed5-6dce-46f5-967f-60c4bdfe8f03	You have a payment waiting for you from the SDP Demo Org. Click https://demo-wallet.stellar.org?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-demo-sdp-backend-dev.stellar.org&name=SDP+Demo+Org&signature=b89ae5713e3eda3c41c453b0b10fb73e46a642a4955e248080363c8a7022c6bed979789fe730f58fe128489eb25aab075c091dd5e46536d246511de47a061207 to register.		2024-07-07 15:34:45.668527+00	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-07-07T15:34:45.668527+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-07-07T15:34:45.668527+00:00\\", \\"status_message\\": null}"}	2024-07-07 15:34:45.668527+00	d8366a73-7fce-44f9-b892-9e80383ba84e
\.


--
-- Data for Name: organizations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.organizations (id, name, timezone_utc_offset, sms_registration_message_template, created_at, updated_at, logo, is_approval_required, otp_message_template, sms_resend_interval, payment_cancellation_period_days) FROM stdin;
98e82622-2d07-4c70-8ed1-739c6302f7ae	SDP Demo Org	+00:00	You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register.	2023-06-02 17:26:12.454669+00	2024-06-25 22:41:09.355985+00	\\x89504e470d0a1a0a0000000d4948445200000150000001500806000000e9e826d9000000017352474200aece1ce900000050655849664d4d002a000000080002011200030000000100010000876900040000000100000026000000000003a00100030000000100010000a00200040000000100000150a0030004000000010000015000000000073307150000015969545874584d4c3a636f6d2e61646f62652e786d7000000000003c783a786d706d65746120786d6c6e733a783d2261646f62653a6e733a6d6574612f2220783a786d70746b3d22584d5020436f726520362e302e30223e0a2020203c7264663a52444620786d6c6e733a7264663d22687474703a2f2f7777772e77332e6f72672f313939392f30322f32322d7264662d73796e7461782d6e7323223e0a2020202020203c7264663a4465736372697074696f6e207264663a61626f75743d22220a202020202020202020202020786d6c6e733a746966663d22687474703a2f2f6e732e61646f62652e636f6d2f746966662f312e302f223e0a2020202020202020203c746966663a4f7269656e746174696f6e3e313c2f746966663a4f7269656e746174696f6e3e0a2020202020203c2f7264663a4465736372697074696f6e3e0a2020203c2f7264663a5244463e0a3c2f783a786d706d6574613e0a195ee10700001e6a494441547801ed9d4d8c244975c75f567557774fafd6bbb0360661049285407c08c937e31b485c6d094e3e7099c31acbf60a63890f236691768c2cb0d62befb292f7601f6c5fb8fa661f912ffe4008c9802c192d78d941c07e687a7ababb2ad3ef65757657755765674465654664fc42eaa9acccf878eff7b2fe199595f126bbfdcc0b85502000010840c099c0c8b9050d2000010840a0248080722240000210f02480807a82a319042000010494730002108080270104d4131ccd2000010820a09c03108000043c0920a09ee068060108400001e51c80000420e0490001f5044733084000020828e700042000014f0208a827389a410002104040390720000108781240403dc1d10c0210800002ca3900010840c0930002ea098e66108000041050ce01084000029e0410504f7034830004208080720e40000210f02480807a82a319042000010494730002108080270104d4131ccd2000010820a09c03108000043c0920a09ee068060108400001e51c80000420e0490001f5044733084000023b208040af048a4cb29e0c286cdcacfcb7270b183676020868ec118cc0fe4c7219173315ca7cc9dab9769980b62ba1b9f637cb76a490fa2f5899d6403e9742c21b470208a82330aabb1330f17cf1e9cf8b1467cb8de753c0e57d6dbccb26f2075ff90b99ea2b0502db2480806e932e7d9704ca99a789673ebd4e641b53409d788e985b5e67cd9ed609d47fc7697d383a84000420301c02cc408713cb703db159e6aa99e6aa7d6d78a13f4c95e395b756d70fb2fe481b46d0470a0410d014a2dcb38f731dd32f3b5715ebeafbb6ecbcf84d4a7f24ba4144db1a927ed2248080a619f78ebd3e57b46d09e6356f2e14548f7436e8352bd8317c02dc031d7e8cf1100210d8120104744b60e9160210183e010474f831c6430840604b0410d02d81a55b084060f80410d0e1c7180f2100812d114040b70436ae6eed97eae1fd5a3d3c8fe23aab52b096c798528872ad8f8b3263db8b8f00cd1b56493f6abb5971b0ecb9976c47f6fca78edecbd82b40b06bb00410d0c186b68963957856af269eb67d29a2975bb6b7d0ac4a538775e62a64a663d6e7f850ff6e3531eab2cee881b639b585ed6ea538956c742493e5e44f37f6916796c569acd6ba0e7863d7541828010474a081dd865b269edf7cfa0baab12a6a4dca7c1950299eeff8c4a7e4c85140b3c9cbf29ef7ff44ecd5a51c1c3f21cfdffd92c8b16336a6d1ae3c79e7ae667142405d78a75c17014d39fa8ebe97198e4c3cf32b69e9eafab109ad0aa789e71b93c7ea6a5e3fb6f7aabc7ef8a6647bf7ae1fbb698f4d9df38642bfd0d7683e655ed8c32604d613e052bb9e0d4720000108d41240406bf17010021080c07a0208e87a361c81000420504b0001adc5c34108400002eb0920a0ebd97004021080402d0104b4160f0721000108ac278080ae67c31108400002b50410d05a3c1c84000420b09e0002ba9e0d4720000108d41240406bf17010021080c07a0208e87a361c81000420504b80b5f0b578867930935cb32acd344b527e9ef1cd16ac5ba9722f55afe77bcac3b6afcaaa54d52f1bd5ff630945262399edbda2afafd6d7bd72341bff4264fa848dea54cef27d917db7b1ca01c8e2e4c499ca22086882678189e78b9655a94c0aa202d748a1b49e260429b32aed686abaa645fb36f17cfb077f2e53135197a2e2f9a31f1e4831fb884b2bd91ddd930ffdf147f4d54d44c9e2e48499ca4a00014df034b099a7149a51a9fc53008d0454eb8d0f34abd2a1bcb1eb985549679e269ed9de0f9c689b59a5789ebccfa9ddd99ec8fdbd9f92c5c9891a957d08700fd4871a6d2000010828010494d30002108080270104d4131ccd2000010820a09c03108000043c0920a09ee068060108400001e51c80000420e0490001f5044733084000020828e700042000014f0208a827b8e89b954fa9ab174d1fa28fde611c8040fb0410d0f69986df23a2197e8cb0300a0208681461c2480840204402ac850f312aa1da347a20d9e465913db7241dbe59952c1393b52df6beef44c477bce2f4514db0e2e69b9361541e1c0104747021dda243e35379cffb7f22af1fbee9368867562513c277bff75853de7ca793f11e3b39d5d4513696be5220d0800002da001255ce09e80d1f9b81667bf79c90f866552a679e2a9e5d65712a4e5fd719e82df50d01750a70c295b9079a70f0711d0210d88c0002ba19bf685a574f2d456330864220020208680441dad4c4c5a7966cdbfe730e0a0420b039010474738671f460aa8972c6112bac8c8600021a4da830140210088d00021a5a44b0070210888600021a4da830140210088d00021a5a44b0070210888600021a4da830140210088d00021a5a44b0070210888600021a4da830140210088d006be1438b88833d99e4322e66322af21b5b15e70f81968f836a5625d93f13a92e9f8b4fdad7f5b42f7296ffba14278fd5d5ba7eace3ac4abe599c6699729c181447ff946da699a927b9db1afa3ccb64968db5651588ebe8d81336010434ecf8d45a67e2f9e29d2f68567915c355a512c672f9d1c253f4fba7f23b9ffd901cedffbc6c651ffe26c5c4f3fbfff3b88ae8db9a54bfa8d3755625dff1764ede213f958fcb585f5dcae1ec813cffd5af6a26a72397667a01db9527efdc95698680ba810ba736021a4e2c9c2d29679e269ef97a01ad3e9a856a64a9a33a4aa13b1f1cfc52d3d2dd2b674e4d07b69967299e27ef6bdaa4acd7795625cb1fea91c569aad6de9ffc9a02729c8196134fa5eb38033538230bccc2b5ad04c63fd1104040a309d52a43f59357d8dfaa63ec830004b64da09aa06c7b1cfaef8100139b1ea03364520410d0a4c28db31080409b0410d03669d21704209014010434a970e32c0420d0260104b44d9af4050108244500014d2adc380b0108b44900016d93267d41000249114040930a37ce4200026d124040dba419555fd5eaf8a88cc6580804458095487d87435712f93ef09ed92aa49ad68b0b94aaed8b575b42c812a6bea3cff8911340407b0ee05c020bcdaa34d59c3c95bc35346aa4c92b0e4eea3474758f9a5569aa09418a935f6938d079b548b22a593211cbc8e448b36c636dcbb5fb0e64c8e2e4006b605511d000026ae2f9cda72dab925b3a344b49f7d1a73e20c7072a180e65e859954c3c7ff4c30329661f71a0a2d72115cf77bff7b84c44e2d2902c4e2eb4865517010d209ee5ccd3c4735d56a57536ea1dec072a9e6f3c726f5d8d95fb879e55a9bc3961e2d951d628b238ad3ccd92d8c98f48498419272100816d104040b741953e210081240820a0498419272100816d104040b741953e2100812408f02352e861ae7b16478ff93e431abadbd80781180830038d214ad808010804498019686861a99b7186662bf640207102cc40433801ca0717f9321e4228b001022e041050175adbaecbec73db84e91f02ad1240405bc5b94967cc4037a1475b08f4410001ed837a83312d47937f9ea6060350050210d898003f226d8c70b30e36fad69e5bfe91776956a5c7dc8c20abd24a5e5d6771ca44f31f8c1eacb4859d71104040fb8e535648a17f577373eade9b2d9b4de47ffffb9df2fadee4e6ba0b357cb30e759de568e8e33d76f4a8fcd73f3b66e05a88239bfd134040fb8f81bf05f9ad72062ae236032df35dee7c47b2bd1f388d5dca7c87598e863e5e76f6369d813a8580ca8111207c81050473200081780820a0f1c40a4b210081c00820a081050473200081780820a0f1c40a4b210081c00820a081050473200081780820a0f1c40a4b210081c00820a081050473200081780820a0f1c40a4b210081c00820a081050473200081780820a0f1c40a4b210081c00820a0810504732000817808b016bec5586592cbb898691a3a4d93d4a4e8626fcb025a66021d1f8a8c6f35697559673292d9de2b2293572ff735d8ca767f2c994716206b636d1ba43959b262f0e38d7f2196f8c495cb59be2fb2ef16bb126c71aaf13b9249c3d3ac0a469e6532cbc66a27f3a68ac9a6afd9ed675e708dfba6630eb6fd4e71262f3efd794dac74d6cc47235fe8c9ace2f98e4f7c4a8e1c05d4c4f3ed1ffcb94c4d441d4a299ef6a17715514d5e22b3b74a61af0e65e8e35d668d7aab031591ddd13d79df6fbea6af6e227a70fc847cfbd9ef891cbb65e192d1ae3c79e7ae4cb35d273ba9bc9e0033d0f56c9c8f64854e097215cf62daac6d29a05a5585d3c4f38d895b56259b799a78ba66556a66dc8a5a26b8360b5d71682bbb2219af0ca34796aab33d91fb7b3fd5f8dd73c76741c8dd53e18d0ab5b6b300babb155b0b04b4c58895e7a5cd28ed13d5a434add7a42fea4000029d1340405b457e7e6947185ba54a67100895007793438d0c76410002c1134040830f1106420002a1124040438d0c76410002c1134040830f1106420002a1124040438d0c76410002c1134040830f1106420002a1124040438d0c76410002c1134040830f1106420002a1124040438d0c76410002c1136025520821b2f5e5939745f6dc924af866399accc67278b22b93e93804efa3b7e1342fe495d12b72a26bdb5d8a6ffc5cc6a0ee760920a0dbe5dbacf7f1a9bce7fd3f91d70fdf6c56ffbcd6459623a756528ae7277fe3e3f2f8034da746d998c06bfbf645ee3bf2cb839f39f5e51b3fa741a8bc550208e856f136ec5c3f7f3603f5cacad37088c56a36f334f17cdb9b8f2cee66db9bc07d998cfe4fe377e4dd030de324c03dd038e386d5108040000410d0008280091080409c0410d038e386d5108040000410d0008280091080409c0410d038e386d5108040000410d0008280091080409c0410d038e386d5108040000410d0008280091080409c0410d038e386d5108040000410d0008280091080409c0410d038e386d51080400004580b1f40107c4df0cdaaf4f88303b1b6947608184b63ea5a4e776672b47726a7e3996b53ea074200010d24103e66584a3a9fac4a73e19df80c499b15040e4f26f2c9777dcc59085fbbf550bef5e37f91d35b08e80aac51ec4240a308d36a23c9aab49a4bd77b2d0e93a9fb0cd4ec24276bd7d16a773cee81b6cb93de200081840820a009051b57210081760920a0edf2a4370840202102086842c1c6550840a05d020868bb3ce90d0210488800029a50b07115021068970002da2e4f7a8300041222808026146c5c850004da258080b6cb93de200081840820a009051b57210081760920a0edf2a4370840202102ac854f28d897ae169249ae7fc5e5ae2d6ed96885d8b53adbe2286d74dd2d9791cc3a8b411b74e8e33a0104f43a9381eda944b212af4c3fb4b97cf73ffe4d25ad9b2c40b98ce5c3bff5dbe7225ad953bd5676f58fbd6b2eaf3e7a2cd93bf3fe1dc7026f0208a837ba481a56fa54e9959a3d9f0fce5440cf3a73c2c65c30e17232bab4b33373560ed43597acbc80050460251576d611e01e681d1d8e41000210a8218080d6c0e110042000813a0208681d1d8e41000210a8218080d6c0e110042000813a0208681d1d8e41000210a821c0aff035700671a8f51f79ab9ff5e774ec97eba195650febbd1b9ef7f5fe7274990002bacc6380efaeca817de47d3ff6da5761fdcdfbbc14cffafe32ab6e55e6cd7423dc529a58efce92f1e69b43f5a5b6bc899f00021a7f0c3bf6c0246651096df5ce0d0585b901108763258080c61a39ecde0a8152eb6fbc225c0ecdb5e192458a5b08688a51c7e7b5044c1011c5b57838708500027a05086fd325500ae7c8eeec369b8296b572e436dd334604014d30fab986fd247b64f95666030ea3622a3bc5898c8a791292cbd5edf58293dbe1fa2a0d46df7e953cdb918772a8b6361445ad96a9e08e4b2e0f35658a6b729608a06c1f7bd42320a05187cfcff8b36c5f9efba7ef3aa712d9d3e19efbf2efca5e7e7431705efec45e2f04968da9e9aceea2e31e36a6b22f7ff38f7e5c9efdf2efc95e71dfc9ea5b8f3e94ff7cf8ef4e6da81c16010434ac787462cdac9c69899c788c362b540c8bf90ccd44d1b66e9aaf9533d008a6a0360335262e5c2aff67d9ae82705b9712c345c5e31449aa09029a54b8db75b694cf72065adfef5c566e92d9fa3e380a81100920a0214625609b1665b0daaeff027ff30c356077310d02b50410d05a3c1c5c45a05c8cb4ea00fb20901801b79b3689c1c1dd6502e5fdbe9ba69bcb4d78078141134040071d5e9c830004b6490001dd26dd21f5ad33cfea9ee790dcc217086c420001dd841e6d210081a40920a049871fe72100814d0820a09bd0a32d04209034010434e9f0e33c0420b009010474137ab48500049226c083f40986dfb2294d5cfdd69fe0ab36961c645eeca1d0ea6f7d874566d7e92b0f9006f8937ee6c1c5dcd055f0655680d90597f52c168f1491245959b499ed650208e8328f24deedc8a9bcf4f5cf489e5542d8cc6d4b67b75b58dab65be70d2eff57a4fa1eacded52f3b5704b5be834e8e1a97bf552e45432e269e961064a469ec8ccba8a8b83433f7f0d123291e5ee5d2ac2db5c22080808611874ead98cf408fbd1fecb47ca2432cae5c4a015d48e767d99c5c4a39936f908cc5a54fea764b80cb5fb7bc190d0210181001b74be6801cc71508b441c092d7174d33d85f1bd01a5fdbc98e880820a011052b6e5387aa147a17d4d3b5f95d60cfc6719f0c83b11e011d4c2871a46b029bff0c8678761db3b6c7e31e68db44e90f0210488600029a4ca871140210689b0002da3651fa8300049221808026136a1c850004da268080b64d94fe200081640820a0c9841a47210081b60920a06d13a53f0840201902086832a1c6510840a06d020868db44e90f0210488600029a4ca871140210689b0002da3651fa8300049221808026136a1c850004da268080b64d94fe200081640820a0c9841a47210081b60920a06d13a53f0840201902086832a1c6510840a06d020868db44e90f0210488600029a4ca871140210689b0002da3651fa8300049221808026136a1c850004da268080b64d94fe2000816408f0bf722613eaf81ccdb3b14c6522f6ea5246c54c76e454ec9502816d124040b74997be372260e279fb732fa814ba9589567fe9eb9fd1d6c76e0da90d01470208a82330aa7747c0669e269e271e4396b356fedb750f72347121c03d50175ad485000420b04000015d80c12604200001170208a80b2dea42000210582080802ec06013021080800b0104d4851675210001082c1040401760b009010840c0850002ea428bba10800004160820a00b30d884000420e042000175a1455d084000020b0410d005186c4200021070218080bad0a22e042000810502ac855f80c16658042c9b922506712dd6864c4caed4a8ef430001f5a1469b4e08584a3acbaae49bceae1323192469020868d2e10fdbf9f90c5453d2915529ec40256c1df740130e3eae4300029b11404037e3476b08402061020868c2c1c775084060330208e866fc680d0108244c00014d38f8b80e01086c460001dd8c1fad210081840920a009071fd7210081cd0820a09bf1a33504209030010434e1e0e33a0420b019010474337eb486000412268080261c7c5c8700043623c05af8cdf8d1ba01814c3219e5fa6f9135a8dd5f95222b241f15baf49ec5f7fd4521ae9111d0b8e215a5b5269efff0e2dfcb380ffb0bcf6c94cbef3ff96999a9885220d0840002da841275362260334f13cfddd978a37eba681cfa2cb90b068cd19c40d85382e67e501302108040e70410d0ce913320042030140208e85022891f108040e70410d0ce913320042030140208e85022891f108040e70410d0ce913320042030140208e85022891f108040e70410d0ce913320042030140208e85022891f108040e70410d0ce91332004203014022ce5dc3492ba4cb14a91315f0658bddbb463da430002a1134040574428935cc6c54c85315f71f4729726efd162027a2e9aa323918313dbe5560e44a6e353b7365afb746726afdd7ae8dc6ea26bd20f4f26329976b336ddb21c59a28ed08bd968b6ba168bc3d1dea99c8e674e4d2d76d6d6b594e78a9e33cea538954ccfd1896328f22c935936d61c557c61bdca3cbbfdcc0bee67ccd55e06f67ea73893179ffebc88bed6967301bda8b37f261f7dea03727cf08b8b5d4d36ec03f17072e42ca27321dc7516c2c71f1cc827dff53179fcc8e753d8c4a3e53a768119723abbd70e8fe55b2fffab5ecc8e971dbfe1dd5c78cf9c8577673691fdd343b157977270fc847cfbd9ef891cbbb593d1ae3c79e7ae4cb35d97e192a8cb0c744598b3422fd1b98a67315d717461d7d54b8f5ea01fa878bef1c8bd854adbdbb419cfe92df7198c59e43a5bdac40bcbaf39e41471c6d2c4f3dea3fa0da4836217dcfb07eedf584ad3ecdb51eede7654e8c9eefacdaa03167d0f8180ae8840799e14aa86570572455d76410002e912404057c6fefc52db4440b92aaf24c84e08a44080bbc22944191f210081ad104040b782954e210081140820a02944191f210081ad104040b782954e210081140820a02944191f210081ad104040b782954e210081140820a02944191f210081ad104040b782954e210081140820a02944191f210081ad106025d20aac967dc61228342a8bd97b74fdfcade3b73a2f19f64d26e29b54e2509349b826a23016b966e499caa47c6dc4e6bcd248335bedc8a9d86bc8c5d7bf3c1fcbc1c35f95c7468f38b9d775dc2d998814afeab9ed9e4ca4fc4c38799746650474459c2d7597659f291328ac38bebceb72bd67367a20cfdffd927bd2054d8af4e1cfbec539418465e4f9ee5ffd52b3eb2c5b74e33bcd91f2e9afd987dd2da98489e7edcfbde0d84ab495c84b5fff8cbeba1a7aa327ad56f0f5ef4cbfc7e57b6a8a6b76c0aee3aee2f9875f7c468afcd0895b95cecea951229511d01581b6bc87d34c3f158eebdc27b936b054611ed96e7c6684651bd3a4fb2b9ca8d9659ff5f9786e026a33346ba1194f9d8bb50d3d398bb77f965fd3f3dad065dc6de669e279ea3a03758e763a0db8079a4eacf114021068990002da3250ba830004d2218080a6136b3c8500045a268080b60c94ee200081740820a0e9c41a4f2100819609f02b7ccb40bdbb5bf8ef9197fbb87c4c6a793fef200081be0920a07d47a07c56aad07f4792d9ffc3543deb533ea07f9378de74bc77e73000028326c057f860c27b2986f67f935fbe5b6fa0e363aaeb3be2080420e0458019a817b6f61a652a9573b1d4a7b1b399d833d9564c1c6d5e6ac729108040980410d09ee352c9e3c5acf37c5a79f1df70ebbdd1cba2b517df5e1e602b11025c50c30a3402da773c2c71897d65d797f24fed318db4b96776553cedc885b26aa5a5e37d3bc2f8db21702999975b7a21d59b6fa3d1c23df30683e7233bb3aa4b76830654b99100027a23a2e6159cb2382d76ab6be7f735534e5ecaa6fd6b9a6a6bc76dab2ad74ffc5bc78f8bcc34bb4e47c5b22939e6f1292db393cc12753c74cc56d4915b17c39c64ba4edcde75b556bc38953243d28505cb1b760b675eaa57db53c8e1f15b74f73dc9c71a0dbb00372dd9ae9e52fcecd11457937ad9ed675eb8fec96cd2923ad708642a8163159966599cf4735acceb17e307f2dcd7be32cfe663d1a82252bd5e1be97c876587b34ffccce143a4d5f77480bffbc61fc97eee9685c437dd9b89e7937ffa0d15d1b08ba17ceae9afc9c968bf1343b3d1915ff62ebd51fed49ffd79991864aa495af286a268f58a1d4b478888b6156066a06d91d47e5cb3385502ba5b6671d20f6d71a67fe73933cbd9e75561bc49515b74664557f319a8a61d7234c3669e269e3e599c5698b1bd5d3af334f1ec2a5bd1a4cce2e491bd2bd38f6df1889ce90c74aadb4e828878b67afe20a0ade274eb6c9ea476acb7b35434eddea6ce482fc569954aaddae73626b50740404570a633c95979ab419f1fbef8aa7fb36fd7cfa0eb7b6eee851a150104b422d1cbab7ee9d77b5845f500bd89e852b9fa7ee9206f1226303311d5bfe51f1a1306d293eb08684fe0191602ed10e022db0e47bf5e10503f6eb482401004ec56f9d53be5eb0c436ad791f1df8f80fab3a3250482208030f617069e67e88f3d2343000291134040fb0ea07e079bff10d0f48b58df06333e0420501140402b123dbd229b3d81675808b44000016d01225d40000269124040d38c3b5e4300022d1040405b8048171080409a04788ca9e7b8db2328e5924e5bdf3c727d2045ebdbda795b021a70f1cde264346c0dbdb377b6de5b936c347f42f21cde68771e8bf3b7db7ef1ceded5b19ddbe61073ff64630a207a96bd695c4ccb54652ee6ec6af291bfbef34555184d42e250f6b4ae4f3626872196aafa66713acb0ee4f667ff729e626ea9c71bdea8c0fcc99dbb72a6e9db5c8a09da4c85d792c274515cb3775536756d67352eafd7093003bdcea4f33df681c81d3feca591e5d42cfcdff17db338998ef9799795e2d9555625df13c6357b97ef38b4db1e816e2eb5dbb39f9e21000108f4460001ed0d3d03430002b1134040638f20f6430002bd1140407b43cfc0108040ec0410d0d82388fd1080406f0410d0ded033300420103b010434f608623f0420d01b0104b437f40c0c0108c44e00018d3d82d80f0108f4460001ed0d3d03430002b1134040638f20f6430002bd1140407b43cfc0108040ec0410d0d82388fd1080406f0410d0ded033300420103b010434f608623f0420d01b0104b437f40c0c0108c44e00018d3d82d80f0108f4460001ed0d3d03430002b1134040638f20f6430002bd1140407b43cfc0108040ec0410d0d82388fd1080406f0410d0ded033300420103b010434f608623f0420d01b0104b437f40c0c0108c44e00018d3d82d80f0108f4460001ed0d3d03430002b113f87fee96129c4624f8260000000049454e44ae426082	f	{{.OTP}} is your {{.OrganizationName}} phone verification code.	2	5
\.


--
-- Data for Name: payments; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.payments (id, stellar_transaction_id, created_at, receiver_id, disbursement_id, amount, asset_id, stellar_operation_id, blockchain_sender_id, status, status_history, updated_at, receiver_wallet_id, external_payment_id) FROM stdin;
825e8c3c-4d5e-412f-8d98-eefdc0da7a19	\N	2024-07-03 15:34:39.618582+00	179c2ed5-6dce-46f5-967f-60c4bdfe8f03	f1e46a04-dbf6-49f7-a928-271a19fa1fd5	500.0000000	4c62168d-b092-4073-b1c2-0e4c19377188	\N	\N	CANCELED	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-07-03T15:34:39.618582+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-07-03T15:34:39.819203+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"CANCELED\\", \\"timestamp\\": \\"2024-07-08T15:34:40.522598+00:00\\", \\"status_message\\": null}"}	2024-07-08 15:34:40.522598+00	d8366a73-7fce-44f9-b892-9e80383ba84e	\N
ba278017-8bd6-4282-8963-e6257badea63	\N	2024-06-12 16:37:07.976312+00	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	760a8542-c656-42ba-bcb1-397b24c4f744	0.1000000	4c62168d-b092-4073-b1c2-0e4c19377188	\N	\N	CANCELED	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"CANCELED\\", \\"timestamp\\": \\"2024-06-25T22:44:40.520233+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:44:40.520233+00	c08eea5f-0c25-4611-a869-31bf4844b468	\N
962452e2-6e9d-4246-b05c-1fdd9ab8bd03	\N	2024-06-12 16:37:07.976312+00	560177da-47a7-4e1c-8361-757fc3ba4c7f	760a8542-c656-42ba-bcb1-397b24c4f744	0.1000000	4c62168d-b092-4073-b1c2-0e4c19377188	\N	\N	CANCELED	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"CANCELED\\", \\"timestamp\\": \\"2024-06-25T22:44:40.520233+00:00\\", \\"status_message\\": null}"}	2024-06-25 22:44:40.520233+00	8ec2ff7b-8193-420b-ac40-698dc4d0f139	\N
9336b377-8922-4014-868d-9a2bad8e9ef4	b566fa278958aeac1f457f7f13e4adc6b3037a2099b8ab4941e48c3b94e476e1	2024-06-12 16:37:07.976312+00	677820fb-68c4-4595-b0ec-ffa7df5390ef	760a8542-c656-42ba-bcb1-397b24c4f744	0.1000000	4c62168d-b092-4073-b1c2-0e4c19377188	\N	\N	SUCCESS	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"PENDING\\", \\"timestamp\\": \\"2024-06-12T16:37:50.525664+00:00\\", \\"status_message\\": null}","{\\"status\\": \\"SUCCESS\\", \\"timestamp\\": \\"2024-06-12T16:42:00.528987+00:00\\", \\"status_message\\": \\"\\"}"}	2024-06-12 16:42:00.528987+00	5a7476b8-df72-4663-b2b3-5031829e04e4	\N
\.


--
-- Data for Name: receiver_verifications; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.receiver_verifications (receiver_id, verification_field, hashed_value, attempts, created_at, updated_at, confirmed_at, failed_at) FROM stdin;
b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	DATE_OF_BIRTH	$2a$04$J/WuvEtcrPp/n6f2cFsL3Ork1BVotzFtnPQHskHN910pzruUbkK9e	0	2024-06-12 16:37:07.976312+00	2024-06-12 16:37:07.976312+00	\N	\N
560177da-47a7-4e1c-8361-757fc3ba4c7f	DATE_OF_BIRTH	$2a$04$A5wJK51k1swfxXEgzmRYJuWeC3/c15cp3pnO5OCJG8tPAHFxypTYW	0	2024-06-12 16:37:07.976312+00	2024-06-12 16:37:07.976312+00	\N	\N
677820fb-68c4-4595-b0ec-ffa7df5390ef	DATE_OF_BIRTH	$2a$04$llSDbJHAgzOL9yp/6L8Jte7mjg/0kcGfzGYx.d4IIe7oLk.uhM9Lu	0	2024-06-12 16:37:07.976312+00	2024-06-12 16:37:47.043759+00	2024-06-12 16:37:47.0469+00	\N
179c2ed5-6dce-46f5-967f-60c4bdfe8f03	DATE_OF_BIRTH	$2a$04$U7sASLgTOUkHAZSWCUO24uokOK0VfPRZSc0VzGSosk5Ushyb8V1Ki	3	2024-07-03 15:34:39.618582+00	2024-07-03 15:51:12.903247+00	\N	2024-07-03 15:51:12.888708+00
\.


--
-- Data for Name: receiver_wallets; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.receiver_wallets (id, receiver_id, wallet_id, stellar_address, stellar_memo, stellar_memo_type, created_at, updated_at, status, status_history, otp, otp_created_at, otp_confirmed_at, anchor_platform_transaction_id, invitation_sent_at, anchor_platform_transaction_synced_at) FROM stdin;
c08eea5f-0c25-4611-a869-31bf4844b468	b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	7a0c5a0a-33c1-42b9-a27b-d657567c2925	\N	\N	\N	2024-06-12 16:37:07.976312+00	2024-06-12 16:37:10.908561+00	READY	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\"}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\"}"}	\N	\N	\N	\N	2024-06-12 16:37:10.908561+00	\N
8ec2ff7b-8193-420b-ac40-698dc4d0f139	560177da-47a7-4e1c-8361-757fc3ba4c7f	7a0c5a0a-33c1-42b9-a27b-d657567c2925	\N	\N	\N	2024-06-12 16:37:07.976312+00	2024-06-12 16:37:10.908561+00	READY	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\"}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\"}"}	\N	\N	\N	\N	2024-06-12 16:37:10.908561+00	\N
5a7476b8-df72-4663-b2b3-5031829e04e4	677820fb-68c4-4595-b0ec-ffa7df5390ef	7a0c5a0a-33c1-42b9-a27b-d657567c2925	GBBMN6CHPVLHATN6B7GP2KYOCTLOGF4IML7WRL4VMIOXXJNSV5GCUDAB	\N	\N	2024-06-12 16:37:07.976312+00	2024-06-12 16:42:10.550569+00	REGISTERED	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-06-12T16:37:07.976312+00:00\\"}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-06-12T16:37:08.210896+00:00\\"}"}	422610	2024-06-12 16:37:27.818973+00	2024-06-12 16:37:47.054832+00	2c9b47ba-da5e-426d-a4ca-bbf722e1877e	2024-06-12 16:37:10.908561+00	2024-06-12 16:42:10.550569+00
d8366a73-7fce-44f9-b892-9e80383ba84e	179c2ed5-6dce-46f5-967f-60c4bdfe8f03	79308ea6-da07-4520-9db4-1b9b390d5d7e	\N	\N	\N	2024-07-03 15:34:39.618582+00	2024-07-03 15:43:31.471024+00	READY	{"{\\"status\\": \\"DRAFT\\", \\"timestamp\\": \\"2024-07-03T15:34:39.618582+00:00\\"}","{\\"status\\": \\"READY\\", \\"timestamp\\": \\"2024-07-03T15:34:39.819203+00:00\\"}"}	487190	2024-07-03 15:43:31.471024+00	\N	\N	2024-07-03 15:34:40.68967+00	\N
\.


--
-- Data for Name: receivers; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.receivers (id, created_at, phone_number, email, updated_at, external_id) FROM stdin;
677820fb-68c4-4595-b0ec-ffa7df5390ef	2024-06-12 16:37:07.976312+00	+14155555555	\N	2024-06-12 16:37:07.976312+00	internal-id-A
b24fd41a-5cb9-4f3e-be5a-a6d2a657da4e	2024-06-12 16:37:07.976312+00	+5548999999999	\N	2024-06-12 16:37:07.976312+00	internal-id-B
560177da-47a7-4e1c-8361-757fc3ba4c7f	2024-06-12 16:37:07.976312+00	+14154444444	\N	2024-06-12 16:37:07.976312+00	internal-id-C
179c2ed5-6dce-46f5-967f-60c4bdfe8f03	2024-07-03 15:34:39.618582+00	+14153333333	\N	2024-07-03 15:34:39.618582+00	internal-id-D
\.


--
-- Data for Name: submitter_transactions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.submitter_transactions (id, external_id, status_message, asset_code, asset_issuer, amount, destination, created_at, started_at, sent_at, completed_at, stellar_transaction_hash, attempts_count, synced_at, updated_at, xdr_sent, xdr_received, locked_at, locked_until_ledger_number, status, status_history) FROM stdin;
048c2c23-dc15-49ad-ba8b-c91f2307c9f0	a9498205-8d8b-45b0-bcd9-57dc0058d815	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-04-18 14:54:04.654075+00	\N	2024-04-18 14:54:22.595344+00	2024-04-18 14:54:28.073046+00	67d8a6a01d1229502ee6a76e7e9223fd79781749d7f5140ceddc4963d3df12d0	1	2024-04-18 14:54:34.65445+00	2024-04-18 14:54:34.65445+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiE1SgAAAAEAAAAAABIIlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIbcpxhDfULSGMy3NETWeLaW4xJukWfD5zzM98bgWTzzyYQHGL0r5ZjpHjfLsbYD2WA0U1gDXtFeMk5NQ9KHfB8/WmRgAAABADC+o4DldeMrPwWD9tFLmgxeqHMX32vSHIAHSaxB9fsjkUlUrLhYJ0uc27f0xZ8K1MyuoxOALY4cOVS7dAmlpBAAAAAAAAAAB7OKZaQAAAECy15xyqhXiwYclSTqCZoa/PwynhjQ5W5a0xd9imra48mvhGvPxo677woWC4etE3XDCybUG+wcuxLcnAU6g8XIN	AAAAAAAAAMgAAAABtdvpZSvBEj6fiHnxAPEw/8WB2P0zetjvEyeYEACoOGkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-04-18T14:54:04.654075+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiE1SgAAAAEAAAAAABIIlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIbcpxhDfULSGMy3NETWeLaW4xJukWfD5zzM98bgWTzzyYQHGL0r5ZjpHjfLsbYD2WA0U1gDXtFeMk5NQ9KHfB8/WmRgAAABADC+o4DldeMrPwWD9tFLmgxeqHMX32vSHIAHSaxB9fsjkUlUrLhYJ0uc27f0xZ8K1MyuoxOALY4cOVS7dAmlpBAAAAAAAAAAB7OKZaQAAAECy15xyqhXiwYclSTqCZoa/PwynhjQ5W5a0xd9imra48mvhGvPxo677woWC4etE3XDCybUG+wcuxLcnAU6g8XIN\\", \\"timestamp\\": \\"2024-04-18T14:54:22.595344+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"67d8a6a01d1229502ee6a76e7e9223fd79781749d7f5140ceddc4963d3df12d0\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiE1SgAAAAEAAAAAABIIlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIbcpxhDfULSGMy3NETWeLaW4xJukWfD5zzM98bgWTzzyYQHGL0r5ZjpHjfLsbYD2WA0U1gDXtFeMk5NQ9KHfB8/WmRgAAABADC+o4DldeMrPwWD9tFLmgxeqHMX32vSHIAHSaxB9fsjkUlUrLhYJ0uc27f0xZ8K1MyuoxOALY4cOVS7dAmlpBAAAAAAAAAAB7OKZaQAAAECy15xyqhXiwYclSTqCZoa/PwynhjQ5W5a0xd9imra48mvhGvPxo677woWC4etE3XDCybUG+wcuxLcnAU6g8XIN\\", \\"timestamp\\": \\"2024-04-18T14:54:28.07135+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABtdvpZSvBEj6fiHnxAPEw/8WB2P0zetjvEyeYEACoOGkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"67d8a6a01d1229502ee6a76e7e9223fd79781749d7f5140ceddc4963d3df12d0\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiE1SgAAAAEAAAAAABIIlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIbcpxhDfULSGMy3NETWeLaW4xJukWfD5zzM98bgWTzzyYQHGL0r5ZjpHjfLsbYD2WA0U1gDXtFeMk5NQ9KHfB8/WmRgAAABADC+o4DldeMrPwWD9tFLmgxeqHMX32vSHIAHSaxB9fsjkUlUrLhYJ0uc27f0xZ8K1MyuoxOALY4cOVS7dAmlpBAAAAAAAAAAB7OKZaQAAAECy15xyqhXiwYclSTqCZoa/PwynhjQ5W5a0xd9imra48mvhGvPxo677woWC4etE3XDCybUG+wcuxLcnAU6g8XIN\\", \\"timestamp\\": \\"2024-04-18T14:54:28.073046+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABtdvpZSvBEj6fiHnxAPEw/8WB2P0zetjvEyeYEACoOGkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"67d8a6a01d1229502ee6a76e7e9223fd79781749d7f5140ceddc4963d3df12d0\\"}"}
b8bcf39c-65a1-4c1f-8e2b-65dbf6ab2374	84dd1a36-8396-4d0f-9c46-f8102a16058b	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-02-23 21:01:47.278919+00	\N	2024-02-23 21:01:59.079888+00	2024-02-23 21:02:02.750159+00	7d0bb1a7ac8487a9a4d7cc518fc811fcaf7a27be45d21794a87ae92253220f91	1	2024-02-23 21:02:07.260842+00	2024-02-23 21:02:07.260842+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAClQIC/e947X6e9QKlO2w41w+5Zlpq8+NNlwux92C/i5gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkI8wAAAAEAAAAAAARNAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1AAqvQpJr/Lnd1Yf1gA8sDyy7ifRp+IpOJ3J2H688UCIOcGxlhyhWJTTIMb4nHmvlpl0yOKDDkfCJ3paqjtmDtgv4uYAAABAhxSDvsXe96MdMMWF96xe3RRAG+pEBABU9MA8Goe/HNczBhx1WTidKU4H7BGmjEgqO2Bvz8BeOTWzzS45M1pCAwAAAAAAAAAB7OKZaQAAAEBCfEZVAaLK65Nimur9EXYUvZvR2PDVxokkr5co1l2aOvJze61Ohuj7yYgNOSPC16gmEVUsc1gizEu74MDR/HMJ	AAAAAAAAAMgAAAAB0FDlaRoY08DU7/ykmfJCBzYU4DgWAAjpuUVH9nfo0qoAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-02-23T21:01:47.278919+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAClQIC/e947X6e9QKlO2w41w+5Zlpq8+NNlwux92C/i5gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkI8wAAAAEAAAAAAARNAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1AAqvQpJr/Lnd1Yf1gA8sDyy7ifRp+IpOJ3J2H688UCIOcGxlhyhWJTTIMb4nHmvlpl0yOKDDkfCJ3paqjtmDtgv4uYAAABAhxSDvsXe96MdMMWF96xe3RRAG+pEBABU9MA8Goe/HNczBhx1WTidKU4H7BGmjEgqO2Bvz8BeOTWzzS45M1pCAwAAAAAAAAAB7OKZaQAAAEBCfEZVAaLK65Nimur9EXYUvZvR2PDVxokkr5co1l2aOvJze61Ohuj7yYgNOSPC16gmEVUsc1gizEu74MDR/HMJ\\", \\"timestamp\\": \\"2024-02-23T21:01:59.079888+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"7d0bb1a7ac8487a9a4d7cc518fc811fcaf7a27be45d21794a87ae92253220f91\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAClQIC/e947X6e9QKlO2w41w+5Zlpq8+NNlwux92C/i5gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkI8wAAAAEAAAAAAARNAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1AAqvQpJr/Lnd1Yf1gA8sDyy7ifRp+IpOJ3J2H688UCIOcGxlhyhWJTTIMb4nHmvlpl0yOKDDkfCJ3paqjtmDtgv4uYAAABAhxSDvsXe96MdMMWF96xe3RRAG+pEBABU9MA8Goe/HNczBhx1WTidKU4H7BGmjEgqO2Bvz8BeOTWzzS45M1pCAwAAAAAAAAAB7OKZaQAAAEBCfEZVAaLK65Nimur9EXYUvZvR2PDVxokkr5co1l2aOvJze61Ohuj7yYgNOSPC16gmEVUsc1gizEu74MDR/HMJ\\", \\"timestamp\\": \\"2024-02-23T21:02:02.736434+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB0FDlaRoY08DU7/ykmfJCBzYU4DgWAAjpuUVH9nfo0qoAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"7d0bb1a7ac8487a9a4d7cc518fc811fcaf7a27be45d21794a87ae92253220f91\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAClQIC/e947X6e9QKlO2w41w+5Zlpq8+NNlwux92C/i5gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkI8wAAAAEAAAAAAARNAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1AAqvQpJr/Lnd1Yf1gA8sDyy7ifRp+IpOJ3J2H688UCIOcGxlhyhWJTTIMb4nHmvlpl0yOKDDkfCJ3paqjtmDtgv4uYAAABAhxSDvsXe96MdMMWF96xe3RRAG+pEBABU9MA8Goe/HNczBhx1WTidKU4H7BGmjEgqO2Bvz8BeOTWzzS45M1pCAwAAAAAAAAAB7OKZaQAAAEBCfEZVAaLK65Nimur9EXYUvZvR2PDVxokkr5co1l2aOvJze61Ohuj7yYgNOSPC16gmEVUsc1gizEu74MDR/HMJ\\", \\"timestamp\\": \\"2024-02-23T21:02:02.750159+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB0FDlaRoY08DU7/ykmfJCBzYU4DgWAAjpuUVH9nfo0qoAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"7d0bb1a7ac8487a9a4d7cc518fc811fcaf7a27be45d21794a87ae92253220f91\\"}"}
12670c3a-7028-400b-8407-dd2c1a9c3b26	6e2e93fe-2e2b-4d5d-b896-50811ecff519	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-06-06 21:18:50.516389+00	\N	2024-06-06 21:18:54.051283+00	2024-06-06 21:18:55.791184+00	f272d174bd519345ce948f037e692c0177b65ab81d846f95847824c31f560bbc	1	2024-06-06 21:19:00.513812+00	2024-06-06 21:19:00.513812+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABiCL2u3jniLJEZELecVq/Nui5SjpgCIMff9KJ9QsPlvQABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmIo6gAAAAEAAAAAAB5kdgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABlG+ZsrmwfzNFnAZ+DhgUWD6dfPgjpMwaejxhYKeh58wVLGK84FOOOwNrS1Fr9g7UfRF5iiVUCQSTwBEA9ePDELD5b0AAABA84EqRX+uFBtfs/mQ46mbKA+6etbCQqPAnqRPNeCQ/abLJ5oequTNJ7nWNHwRddBTZNu6rxYYcF1kVjDiQFK/BQAAAAAAAAAB7OKZaQAAAEAkE23MF0O+q/0ST0oGiwB89X+YwqoqU79qSW7DFLSZh9T4e4ZtAPOPEr1u+oOADM7iJUmG4iAVZU2Y3u0X3y0O	AAAAAAAAAMgAAAAB1LI/eA4N42EQURIVP5oBNg+E8rm47Y1PBTqIrB0moqgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-06T21:18:50.516389+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABiCL2u3jniLJEZELecVq/Nui5SjpgCIMff9KJ9QsPlvQABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmIo6gAAAAEAAAAAAB5kdgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABlG+ZsrmwfzNFnAZ+DhgUWD6dfPgjpMwaejxhYKeh58wVLGK84FOOOwNrS1Fr9g7UfRF5iiVUCQSTwBEA9ePDELD5b0AAABA84EqRX+uFBtfs/mQ46mbKA+6etbCQqPAnqRPNeCQ/abLJ5oequTNJ7nWNHwRddBTZNu6rxYYcF1kVjDiQFK/BQAAAAAAAAAB7OKZaQAAAEAkE23MF0O+q/0ST0oGiwB89X+YwqoqU79qSW7DFLSZh9T4e4ZtAPOPEr1u+oOADM7iJUmG4iAVZU2Y3u0X3y0O\\", \\"timestamp\\": \\"2024-06-06T21:18:54.051283+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"f272d174bd519345ce948f037e692c0177b65ab81d846f95847824c31f560bbc\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABiCL2u3jniLJEZELecVq/Nui5SjpgCIMff9KJ9QsPlvQABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmIo6gAAAAEAAAAAAB5kdgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABlG+ZsrmwfzNFnAZ+DhgUWD6dfPgjpMwaejxhYKeh58wVLGK84FOOOwNrS1Fr9g7UfRF5iiVUCQSTwBEA9ePDELD5b0AAABA84EqRX+uFBtfs/mQ46mbKA+6etbCQqPAnqRPNeCQ/abLJ5oequTNJ7nWNHwRddBTZNu6rxYYcF1kVjDiQFK/BQAAAAAAAAAB7OKZaQAAAEAkE23MF0O+q/0ST0oGiwB89X+YwqoqU79qSW7DFLSZh9T4e4ZtAPOPEr1u+oOADM7iJUmG4iAVZU2Y3u0X3y0O\\", \\"timestamp\\": \\"2024-06-06T21:18:55.789591+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB1LI/eA4N42EQURIVP5oBNg+E8rm47Y1PBTqIrB0moqgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"f272d174bd519345ce948f037e692c0177b65ab81d846f95847824c31f560bbc\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABiCL2u3jniLJEZELecVq/Nui5SjpgCIMff9KJ9QsPlvQABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmIo6gAAAAEAAAAAAB5kdgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABlG+ZsrmwfzNFnAZ+DhgUWD6dfPgjpMwaejxhYKeh58wVLGK84FOOOwNrS1Fr9g7UfRF5iiVUCQSTwBEA9ePDELD5b0AAABA84EqRX+uFBtfs/mQ46mbKA+6etbCQqPAnqRPNeCQ/abLJ5oequTNJ7nWNHwRddBTZNu6rxYYcF1kVjDiQFK/BQAAAAAAAAAB7OKZaQAAAEAkE23MF0O+q/0ST0oGiwB89X+YwqoqU79qSW7DFLSZh9T4e4ZtAPOPEr1u+oOADM7iJUmG4iAVZU2Y3u0X3y0O\\", \\"timestamp\\": \\"2024-06-06T21:18:55.791184+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB1LI/eA4N42EQURIVP5oBNg+E8rm47Y1PBTqIrB0moqgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"f272d174bd519345ce948f037e692c0177b65ab81d846f95847824c31f560bbc\\"}"}
767a2b1f-dc3d-417d-aaef-4aafe0795e9f	e60a06a2-92e5-4a53-8dee-10677a0efc11	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-02-23 21:17:23.549317+00	\N	2024-02-23 21:17:41.521762+00	2024-02-23 21:17:47.785902+00	266bdcc3fe18fad98390da1cdda4aac80562a12428529da3a70077f32b0ccefc	1	2024-02-23 21:17:53.549034+00	2024-02-23 21:17:53.549034+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA0ItZNO63siMmrGBguUNDwhf3SQIKJwo7nUAMkZlgQQQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkMoQAAAAEAAAAAAARNtgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA/LITDNi4swQIFtL57leslk4W4jKt4JgM3+OR4OcuavBM0H2YkLsNuInlXBhI2WdqcK1jdE1tnc4xdo233OhiB2ZYEEEAAABA1aFKyLlbdgDq+XyESbEbRfo4mMmuJHlL6BUD61AVXFIN/UeXiWuwScAdG7XZi02fAUCtlaGlp/7bjSaDIozMAAAAAAAAAAAB7OKZaQAAAEAOCfEXi7ma+y0NSG4yOKB3F8I1OkkCc5Sj0OuOgiD4MHnd48bYyEI0YAdBsPkacV3rUwzK7v08bM5X4gRbDkYK	AAAAAAAAAMgAAAAB7uXreqL6MQa3wIjFBvqpDNhUw+cBeOl1CXSHoq6S5I8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-02-23T21:17:23.549317+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA0ItZNO63siMmrGBguUNDwhf3SQIKJwo7nUAMkZlgQQQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkMoQAAAAEAAAAAAARNtgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA/LITDNi4swQIFtL57leslk4W4jKt4JgM3+OR4OcuavBM0H2YkLsNuInlXBhI2WdqcK1jdE1tnc4xdo233OhiB2ZYEEEAAABA1aFKyLlbdgDq+XyESbEbRfo4mMmuJHlL6BUD61AVXFIN/UeXiWuwScAdG7XZi02fAUCtlaGlp/7bjSaDIozMAAAAAAAAAAAB7OKZaQAAAEAOCfEXi7ma+y0NSG4yOKB3F8I1OkkCc5Sj0OuOgiD4MHnd48bYyEI0YAdBsPkacV3rUwzK7v08bM5X4gRbDkYK\\", \\"timestamp\\": \\"2024-02-23T21:17:41.521762+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"266bdcc3fe18fad98390da1cdda4aac80562a12428529da3a70077f32b0ccefc\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA0ItZNO63siMmrGBguUNDwhf3SQIKJwo7nUAMkZlgQQQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkMoQAAAAEAAAAAAARNtgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA/LITDNi4swQIFtL57leslk4W4jKt4JgM3+OR4OcuavBM0H2YkLsNuInlXBhI2WdqcK1jdE1tnc4xdo233OhiB2ZYEEEAAABA1aFKyLlbdgDq+XyESbEbRfo4mMmuJHlL6BUD61AVXFIN/UeXiWuwScAdG7XZi02fAUCtlaGlp/7bjSaDIozMAAAAAAAAAAAB7OKZaQAAAEAOCfEXi7ma+y0NSG4yOKB3F8I1OkkCc5Sj0OuOgiD4MHnd48bYyEI0YAdBsPkacV3rUwzK7v08bM5X4gRbDkYK\\", \\"timestamp\\": \\"2024-02-23T21:17:47.783766+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB7uXreqL6MQa3wIjFBvqpDNhUw+cBeOl1CXSHoq6S5I8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"266bdcc3fe18fad98390da1cdda4aac80562a12428529da3a70077f32b0ccefc\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA0ItZNO63siMmrGBguUNDwhf3SQIKJwo7nUAMkZlgQQQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZdkMoQAAAAEAAAAAAARNtgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA/LITDNi4swQIFtL57leslk4W4jKt4JgM3+OR4OcuavBM0H2YkLsNuInlXBhI2WdqcK1jdE1tnc4xdo233OhiB2ZYEEEAAABA1aFKyLlbdgDq+XyESbEbRfo4mMmuJHlL6BUD61AVXFIN/UeXiWuwScAdG7XZi02fAUCtlaGlp/7bjSaDIozMAAAAAAAAAAAB7OKZaQAAAEAOCfEXi7ma+y0NSG4yOKB3F8I1OkkCc5Sj0OuOgiD4MHnd48bYyEI0YAdBsPkacV3rUwzK7v08bM5X4gRbDkYK\\", \\"timestamp\\": \\"2024-02-23T21:17:47.785902+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB7uXreqL6MQa3wIjFBvqpDNhUw+cBeOl1CXSHoq6S5I8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"266bdcc3fe18fad98390da1cdda4aac80562a12428529da3a70077f32b0ccefc\\"}"}
49c8c13e-e881-4d46-a4c7-5adcba449bbd	8b7fa22c-055e-4fc1-b376-94bbde18ff9b	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.0100000	GBWAUMIIMXNUSZGWWA7AKGBDWEJ7MH7IRS56NTP6V2XS3J3RHUXZI36O	2024-02-27 20:50:23.560547+00	\N	2024-02-27 20:50:41.523333+00	2024-02-27 20:50:46.734293+00	c22f78b2575eed4a6370c12db9dd46c86c2b667be5a0e72bd2d8228dc847d300	1	2024-02-27 20:50:53.550306+00	2024-02-27 20:50:53.550306+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAD0brcNt/rxac0AcXJoVooMOOwMdW10PJpC32Jqmcpa0AABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd5MTQAAAAEAAAAAAAVNYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABsCjEIZdtJZNawPgUYI7ET9h/ojLvmzf6ury2ncT0vlAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAABhqAAAAAAAAAAAuzimWkAAABAbArlpSGXfFLs9dVNrBT0Kjyjb8LJkUP7VgQK5dsiJYX6gJ+oiKxp6MywBMF7yR2KjN4D/wBuSeeJzbJKDaIOBZnKWtAAAABAoHs1IvdXgKvC03ntMzGcjXcBNcRO/YXtd8qd+aGgRo+2xqr3lBc0L5R5DSXK+SkOQe0+XImiSMu+wVvTA41GBwAAAAAAAAAB7OKZaQAAAED4FTpDSjio9vhDo61iLCpm67RCeIy7QZLAEN1tMVELVCteJtLu8V1ZixPjlPIVJ4bxfHkKxBzMWPhEU/0dNO0P	AAAAAAAAAMgAAAABZzFOrfoyu6xoY9bXZ46ypPZK3tyUj8AYRUfLoesOLCAAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-02-27T20:50:23.560547+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAD0brcNt/rxac0AcXJoVooMOOwMdW10PJpC32Jqmcpa0AABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd5MTQAAAAEAAAAAAAVNYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABsCjEIZdtJZNawPgUYI7ET9h/ojLvmzf6ury2ncT0vlAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAABhqAAAAAAAAAAAuzimWkAAABAbArlpSGXfFLs9dVNrBT0Kjyjb8LJkUP7VgQK5dsiJYX6gJ+oiKxp6MywBMF7yR2KjN4D/wBuSeeJzbJKDaIOBZnKWtAAAABAoHs1IvdXgKvC03ntMzGcjXcBNcRO/YXtd8qd+aGgRo+2xqr3lBc0L5R5DSXK+SkOQe0+XImiSMu+wVvTA41GBwAAAAAAAAAB7OKZaQAAAED4FTpDSjio9vhDo61iLCpm67RCeIy7QZLAEN1tMVELVCteJtLu8V1ZixPjlPIVJ4bxfHkKxBzMWPhEU/0dNO0P\\", \\"timestamp\\": \\"2024-02-27T20:50:41.523333+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"c22f78b2575eed4a6370c12db9dd46c86c2b667be5a0e72bd2d8228dc847d300\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAD0brcNt/rxac0AcXJoVooMOOwMdW10PJpC32Jqmcpa0AABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd5MTQAAAAEAAAAAAAVNYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABsCjEIZdtJZNawPgUYI7ET9h/ojLvmzf6ury2ncT0vlAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAABhqAAAAAAAAAAAuzimWkAAABAbArlpSGXfFLs9dVNrBT0Kjyjb8LJkUP7VgQK5dsiJYX6gJ+oiKxp6MywBMF7yR2KjN4D/wBuSeeJzbJKDaIOBZnKWtAAAABAoHs1IvdXgKvC03ntMzGcjXcBNcRO/YXtd8qd+aGgRo+2xqr3lBc0L5R5DSXK+SkOQe0+XImiSMu+wVvTA41GBwAAAAAAAAAB7OKZaQAAAED4FTpDSjio9vhDo61iLCpm67RCeIy7QZLAEN1tMVELVCteJtLu8V1ZixPjlPIVJ4bxfHkKxBzMWPhEU/0dNO0P\\", \\"timestamp\\": \\"2024-02-27T20:50:46.731974+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABZzFOrfoyu6xoY9bXZ46ypPZK3tyUj8AYRUfLoesOLCAAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"c22f78b2575eed4a6370c12db9dd46c86c2b667be5a0e72bd2d8228dc847d300\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAD0brcNt/rxac0AcXJoVooMOOwMdW10PJpC32Jqmcpa0AABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd5MTQAAAAEAAAAAAAVNYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABsCjEIZdtJZNawPgUYI7ET9h/ojLvmzf6ury2ncT0vlAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAABhqAAAAAAAAAAAuzimWkAAABAbArlpSGXfFLs9dVNrBT0Kjyjb8LJkUP7VgQK5dsiJYX6gJ+oiKxp6MywBMF7yR2KjN4D/wBuSeeJzbJKDaIOBZnKWtAAAABAoHs1IvdXgKvC03ntMzGcjXcBNcRO/YXtd8qd+aGgRo+2xqr3lBc0L5R5DSXK+SkOQe0+XImiSMu+wVvTA41GBwAAAAAAAAAB7OKZaQAAAED4FTpDSjio9vhDo61iLCpm67RCeIy7QZLAEN1tMVELVCteJtLu8V1ZixPjlPIVJ4bxfHkKxBzMWPhEU/0dNO0P\\", \\"timestamp\\": \\"2024-02-27T20:50:46.734293+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABZzFOrfoyu6xoY9bXZ46ypPZK3tyUj8AYRUfLoesOLCAAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"c22f78b2575eed4a6370c12db9dd46c86c2b667be5a0e72bd2d8228dc847d300\\"}"}
c9907d67-ef34-4859-bd0d-e56df8463844	c29fe338-273c-4906-8aad-d48d10ee862d	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDJUCRN3DXK2JXDS7SVII2UDACR5L2LQUOBLO5H6PYIIK62J37TRVFNN	2024-02-28 20:18:43.56227+00	\N	2024-02-28 20:19:01.524637+00	2024-02-28 20:19:07.736143+00	ae32b4d455a5332bbcc948da4b3369a596b11a72a28e2d4723cf555568e1cd13	1	2024-02-28 20:19:13.560899+00	2024-02-28 20:19:13.560899+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADORp7ouunwVUrrow+mOKF/J+8IWgb1HFabrnTNb3uqpwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd+WYQAAAAEAAAAAAAWMKwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAAjKNArBBQwp+dHPIuKFciP6Gd6ZBbCSRbA68uL4M7yTdUmO1x/MRWd+ds/mQQkUAV6FkQ8SY0vdOMzxYRcAPD297qqcAAABAiS73b+KLeiuaVnYtyeDbTa3oX96f5GOGvoLKM13DZ9hS4OKhzVjIJK5cfFEWPWgMcaqeroEUzoqj1W72Uh4iCgAAAAAAAAAB7OKZaQAAAECzdn9z7XGik4IIOADBbJ9v0xT6zUXtlXh5zMWSAHZ/n309HBQDpaYC0iB9ie+dxjE1aupxaD24VlhNHMx9JNYC	AAAAAAAAAMgAAAABF8I9Mfw5TKx91jvEa8AKP0i/jgiNl+eMalWftvJ0krcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-02-28T20:18:43.56227+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADORp7ouunwVUrrow+mOKF/J+8IWgb1HFabrnTNb3uqpwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd+WYQAAAAEAAAAAAAWMKwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAAjKNArBBQwp+dHPIuKFciP6Gd6ZBbCSRbA68uL4M7yTdUmO1x/MRWd+ds/mQQkUAV6FkQ8SY0vdOMzxYRcAPD297qqcAAABAiS73b+KLeiuaVnYtyeDbTa3oX96f5GOGvoLKM13DZ9hS4OKhzVjIJK5cfFEWPWgMcaqeroEUzoqj1W72Uh4iCgAAAAAAAAAB7OKZaQAAAECzdn9z7XGik4IIOADBbJ9v0xT6zUXtlXh5zMWSAHZ/n309HBQDpaYC0iB9ie+dxjE1aupxaD24VlhNHMx9JNYC\\", \\"timestamp\\": \\"2024-02-28T20:19:01.524637+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"ae32b4d455a5332bbcc948da4b3369a596b11a72a28e2d4723cf555568e1cd13\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADORp7ouunwVUrrow+mOKF/J+8IWgb1HFabrnTNb3uqpwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd+WYQAAAAEAAAAAAAWMKwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAAjKNArBBQwp+dHPIuKFciP6Gd6ZBbCSRbA68uL4M7yTdUmO1x/MRWd+ds/mQQkUAV6FkQ8SY0vdOMzxYRcAPD297qqcAAABAiS73b+KLeiuaVnYtyeDbTa3oX96f5GOGvoLKM13DZ9hS4OKhzVjIJK5cfFEWPWgMcaqeroEUzoqj1W72Uh4iCgAAAAAAAAAB7OKZaQAAAECzdn9z7XGik4IIOADBbJ9v0xT6zUXtlXh5zMWSAHZ/n309HBQDpaYC0iB9ie+dxjE1aupxaD24VlhNHMx9JNYC\\", \\"timestamp\\": \\"2024-02-28T20:19:07.73344+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABF8I9Mfw5TKx91jvEa8AKP0i/jgiNl+eMalWftvJ0krcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"ae32b4d455a5332bbcc948da4b3369a596b11a72a28e2d4723cf555568e1cd13\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADORp7ouunwVUrrow+mOKF/J+8IWgb1HFabrnTNb3uqpwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd+WYQAAAAEAAAAAAAWMKwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAAjKNArBBQwp+dHPIuKFciP6Gd6ZBbCSRbA68uL4M7yTdUmO1x/MRWd+ds/mQQkUAV6FkQ8SY0vdOMzxYRcAPD297qqcAAABAiS73b+KLeiuaVnYtyeDbTa3oX96f5GOGvoLKM13DZ9hS4OKhzVjIJK5cfFEWPWgMcaqeroEUzoqj1W72Uh4iCgAAAAAAAAAB7OKZaQAAAECzdn9z7XGik4IIOADBbJ9v0xT6zUXtlXh5zMWSAHZ/n309HBQDpaYC0iB9ie+dxjE1aupxaD24VlhNHMx9JNYC\\", \\"timestamp\\": \\"2024-02-28T20:19:07.736143+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABF8I9Mfw5TKx91jvEa8AKP0i/jgiNl+eMalWftvJ0krcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"ae32b4d455a5332bbcc948da4b3369a596b11a72a28e2d4723cf555568e1cd13\\"}"}
c15f3cc7-5598-44bd-8c58-1b4391691f73	d272d048-bca2-4be6-abb8-1325e81ad2ce	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	50.0000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-05-01 13:58:22.51672+00	\N	2024-05-01 13:58:32.852212+00	2024-05-01 13:58:35.678616+00	f9f329e7a52038301cb29dcc4f49ed64a15f0a88520f856469683c6c8f9bfe05	1	2024-05-01 13:58:42.507497+00	2024-05-01 13:58:42.507497+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACt5L3ULU+nP8IOSCrlV7hBi9zaLqYxI8IEcajJLAWq6AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJLtAAAAAEAAAAAABVJMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAs3XiWoOw2BJ2zIUrQrFAUuR7qPrzNYLJKhVMVjVwjzZHuUO/SSw0De59KzqAlOiV704L6A8jvkwWeVuKDsY0CiwFqugAAABAu6UVRl47PBbg9/WVyCY55XXRJblmRo3igTfC2NHntjwfX45PxS0q6iVoyzbW51LuXs3inBxUQU1TyD2ccIssCAAAAAAAAAAB7OKZaQAAAEBC8fdskyy5Bvz1rP3JiB7cqr5BKTc4YjlmnXDcNn4WhBOs6clw46OM5wnX7ZbpbYoHg8XWzXVGj3oUSOtJh48A	AAAAAAAAAMgAAAABKon62edkFrwPZYHu226qMC9NEdMDBCvDJrhIHHBAnRUAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-05-01T13:58:22.51672+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACt5L3ULU+nP8IOSCrlV7hBi9zaLqYxI8IEcajJLAWq6AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJLtAAAAAEAAAAAABVJMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAs3XiWoOw2BJ2zIUrQrFAUuR7qPrzNYLJKhVMVjVwjzZHuUO/SSw0De59KzqAlOiV704L6A8jvkwWeVuKDsY0CiwFqugAAABAu6UVRl47PBbg9/WVyCY55XXRJblmRo3igTfC2NHntjwfX45PxS0q6iVoyzbW51LuXs3inBxUQU1TyD2ccIssCAAAAAAAAAAB7OKZaQAAAEBC8fdskyy5Bvz1rP3JiB7cqr5BKTc4YjlmnXDcNn4WhBOs6clw46OM5wnX7ZbpbYoHg8XWzXVGj3oUSOtJh48A\\", \\"timestamp\\": \\"2024-05-01T13:58:32.852212+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"f9f329e7a52038301cb29dcc4f49ed64a15f0a88520f856469683c6c8f9bfe05\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACt5L3ULU+nP8IOSCrlV7hBi9zaLqYxI8IEcajJLAWq6AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJLtAAAAAEAAAAAABVJMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAs3XiWoOw2BJ2zIUrQrFAUuR7qPrzNYLJKhVMVjVwjzZHuUO/SSw0De59KzqAlOiV704L6A8jvkwWeVuKDsY0CiwFqugAAABAu6UVRl47PBbg9/WVyCY55XXRJblmRo3igTfC2NHntjwfX45PxS0q6iVoyzbW51LuXs3inBxUQU1TyD2ccIssCAAAAAAAAAAB7OKZaQAAAEBC8fdskyy5Bvz1rP3JiB7cqr5BKTc4YjlmnXDcNn4WhBOs6clw46OM5wnX7ZbpbYoHg8XWzXVGj3oUSOtJh48A\\", \\"timestamp\\": \\"2024-05-01T13:58:35.676425+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABKon62edkFrwPZYHu226qMC9NEdMDBCvDJrhIHHBAnRUAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"f9f329e7a52038301cb29dcc4f49ed64a15f0a88520f856469683c6c8f9bfe05\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACt5L3ULU+nP8IOSCrlV7hBi9zaLqYxI8IEcajJLAWq6AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJLtAAAAAEAAAAAABVJMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAs3XiWoOw2BJ2zIUrQrFAUuR7qPrzNYLJKhVMVjVwjzZHuUO/SSw0De59KzqAlOiV704L6A8jvkwWeVuKDsY0CiwFqugAAABAu6UVRl47PBbg9/WVyCY55XXRJblmRo3igTfC2NHntjwfX45PxS0q6iVoyzbW51LuXs3inBxUQU1TyD2ccIssCAAAAAAAAAAB7OKZaQAAAEBC8fdskyy5Bvz1rP3JiB7cqr5BKTc4YjlmnXDcNn4WhBOs6clw46OM5wnX7ZbpbYoHg8XWzXVGj3oUSOtJh48A\\", \\"timestamp\\": \\"2024-05-01T13:58:35.678616+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABKon62edkFrwPZYHu226qMC9NEdMDBCvDJrhIHHBAnRUAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"f9f329e7a52038301cb29dcc4f49ed64a15f0a88520f856469683c6c8f9bfe05\\"}"}
e5f781e4-6113-4ad7-b17f-41143e927419	978ea265-13a6-401e-9def-5cfdcac4dea2	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-02-29 02:48:43.550705+00	\N	2024-02-29 02:49:01.528453+00	2024-02-29 02:49:07.786201+00	3e62c6c63939a230281deefb500496ea7d0503d257edd34d90e8df536bd67504	1	2024-02-29 02:49:13.553025+00	2024-02-29 02:49:13.553025+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABg02CcFBNhea+NEyOl6y4Ohaxed3YFMJaee3k+fmxclQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd/xyQAAAAEAAAAAAAWdoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAuumu9WAHVBsWeB2SOyfi+LGdCkU31FIv19shhOgH0yiIzrFnXW72QW56rZFf0xj3pJXtiazAWtIby4FAI8QIC35sXJUAAABA/CI5WxDJQPIV/fkMszCT0FPCFOYznnGkgMiroAbDo50wAcTqx/01tfFm20hhjA0pyGCx/2oZmOeP01wI3mETDgAAAAAAAAAB7OKZaQAAAEBAbgPF7nJ8i9ZhWRoImz2X7djD/+euNp9Xzcsb8EC54JilEBeedN4LnKzq0ZGpymcRC3DyWYnyZ4MNJGYS5DYA	AAAAAAAAAMgAAAAB9MRSiTrZ2RNwgId8MVJ2NbVg4giD7dBr6/42S7615qkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-02-29T02:48:43.550705+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABg02CcFBNhea+NEyOl6y4Ohaxed3YFMJaee3k+fmxclQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd/xyQAAAAEAAAAAAAWdoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAuumu9WAHVBsWeB2SOyfi+LGdCkU31FIv19shhOgH0yiIzrFnXW72QW56rZFf0xj3pJXtiazAWtIby4FAI8QIC35sXJUAAABA/CI5WxDJQPIV/fkMszCT0FPCFOYznnGkgMiroAbDo50wAcTqx/01tfFm20hhjA0pyGCx/2oZmOeP01wI3mETDgAAAAAAAAAB7OKZaQAAAEBAbgPF7nJ8i9ZhWRoImz2X7djD/+euNp9Xzcsb8EC54JilEBeedN4LnKzq0ZGpymcRC3DyWYnyZ4MNJGYS5DYA\\", \\"timestamp\\": \\"2024-02-29T02:49:01.528453+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"3e62c6c63939a230281deefb500496ea7d0503d257edd34d90e8df536bd67504\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABg02CcFBNhea+NEyOl6y4Ohaxed3YFMJaee3k+fmxclQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd/xyQAAAAEAAAAAAAWdoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAuumu9WAHVBsWeB2SOyfi+LGdCkU31FIv19shhOgH0yiIzrFnXW72QW56rZFf0xj3pJXtiazAWtIby4FAI8QIC35sXJUAAABA/CI5WxDJQPIV/fkMszCT0FPCFOYznnGkgMiroAbDo50wAcTqx/01tfFm20hhjA0pyGCx/2oZmOeP01wI3mETDgAAAAAAAAAB7OKZaQAAAEBAbgPF7nJ8i9ZhWRoImz2X7djD/+euNp9Xzcsb8EC54JilEBeedN4LnKzq0ZGpymcRC3DyWYnyZ4MNJGYS5DYA\\", \\"timestamp\\": \\"2024-02-29T02:49:07.784121+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB9MRSiTrZ2RNwgId8MVJ2NbVg4giD7dBr6/42S7615qkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"3e62c6c63939a230281deefb500496ea7d0503d257edd34d90e8df536bd67504\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABg02CcFBNhea+NEyOl6y4Ohaxed3YFMJaee3k+fmxclQABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZd/xyQAAAAEAAAAAAAWdoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAuumu9WAHVBsWeB2SOyfi+LGdCkU31FIv19shhOgH0yiIzrFnXW72QW56rZFf0xj3pJXtiazAWtIby4FAI8QIC35sXJUAAABA/CI5WxDJQPIV/fkMszCT0FPCFOYznnGkgMiroAbDo50wAcTqx/01tfFm20hhjA0pyGCx/2oZmOeP01wI3mETDgAAAAAAAAAB7OKZaQAAAEBAbgPF7nJ8i9ZhWRoImz2X7djD/+euNp9Xzcsb8EC54JilEBeedN4LnKzq0ZGpymcRC3DyWYnyZ4MNJGYS5DYA\\", \\"timestamp\\": \\"2024-02-29T02:49:07.786201+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB9MRSiTrZ2RNwgId8MVJ2NbVg4giD7dBr6/42S7615qkAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"3e62c6c63939a230281deefb500496ea7d0503d257edd34d90e8df536bd67504\\"}"}
dcc20f8b-839d-48aa-8f98-29bb2857684d	1d29022b-5ce6-4b23-9322-7449cd934e7a	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-04-18 13:44:44.65892+00	\N	2024-04-18 13:45:02.59496+00	2024-04-18 13:45:05.070108+00	ff69554c8bc05f99d5021ddde7ff67e24c46f125dc1e4c7da8ac88512cd1b8aa	1	2024-04-18 13:45:14.658954+00	2024-04-18 13:45:14.658954+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABM0TP0rJBU8h1RzaEnx4q6FinHR3M6bzswdOeqUbrj6gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiElCgAAAAEAAAAAABIFfAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAM+nm124s0AcpX4vsR62eHq+PyMjllFs+Wrk9zPZb0oK92EUkALLV8aOKYA8/ZCRPDJ05by+Vy+lBwP90OeYwAVG64+oAAABA7iXuk2rrDI43lbM6ghIWgs96grrF8ygxeNfcEhl2zu3jUnyAHeEbbItINoI3hydraCeBh2SAIdBdBksJJkW/BQAAAAAAAAAB7OKZaQAAAEAp01B28NC4ny+esmWmkTkkJje67noC4l/FAu/bJ/17HEcPwF/mich7jEvenyZduaa6B7zBS4VNyXlBEO1ZbIoF	AAAAAAAAAMgAAAABMPoAd3IKOMLULV4FjmOGR89YnB4jRpI9F9onUOWiEREAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-04-18T13:44:44.65892+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABM0TP0rJBU8h1RzaEnx4q6FinHR3M6bzswdOeqUbrj6gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiElCgAAAAEAAAAAABIFfAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAM+nm124s0AcpX4vsR62eHq+PyMjllFs+Wrk9zPZb0oK92EUkALLV8aOKYA8/ZCRPDJ05by+Vy+lBwP90OeYwAVG64+oAAABA7iXuk2rrDI43lbM6ghIWgs96grrF8ygxeNfcEhl2zu3jUnyAHeEbbItINoI3hydraCeBh2SAIdBdBksJJkW/BQAAAAAAAAAB7OKZaQAAAEAp01B28NC4ny+esmWmkTkkJje67noC4l/FAu/bJ/17HEcPwF/mich7jEvenyZduaa6B7zBS4VNyXlBEO1ZbIoF\\", \\"timestamp\\": \\"2024-04-18T13:45:02.59496+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"ff69554c8bc05f99d5021ddde7ff67e24c46f125dc1e4c7da8ac88512cd1b8aa\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABM0TP0rJBU8h1RzaEnx4q6FinHR3M6bzswdOeqUbrj6gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiElCgAAAAEAAAAAABIFfAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAM+nm124s0AcpX4vsR62eHq+PyMjllFs+Wrk9zPZb0oK92EUkALLV8aOKYA8/ZCRPDJ05by+Vy+lBwP90OeYwAVG64+oAAABA7iXuk2rrDI43lbM6ghIWgs96grrF8ygxeNfcEhl2zu3jUnyAHeEbbItINoI3hydraCeBh2SAIdBdBksJJkW/BQAAAAAAAAAB7OKZaQAAAEAp01B28NC4ny+esmWmkTkkJje67noC4l/FAu/bJ/17HEcPwF/mich7jEvenyZduaa6B7zBS4VNyXlBEO1ZbIoF\\", \\"timestamp\\": \\"2024-04-18T13:45:05.066491+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABMPoAd3IKOMLULV4FjmOGR89YnB4jRpI9F9onUOWiEREAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"ff69554c8bc05f99d5021ddde7ff67e24c46f125dc1e4c7da8ac88512cd1b8aa\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABM0TP0rJBU8h1RzaEnx4q6FinHR3M6bzswdOeqUbrj6gABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiElCgAAAAEAAAAAABIFfAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAM+nm124s0AcpX4vsR62eHq+PyMjllFs+Wrk9zPZb0oK92EUkALLV8aOKYA8/ZCRPDJ05by+Vy+lBwP90OeYwAVG64+oAAABA7iXuk2rrDI43lbM6ghIWgs96grrF8ygxeNfcEhl2zu3jUnyAHeEbbItINoI3hydraCeBh2SAIdBdBksJJkW/BQAAAAAAAAAB7OKZaQAAAEAp01B28NC4ny+esmWmkTkkJje67noC4l/FAu/bJ/17HEcPwF/mich7jEvenyZduaa6B7zBS4VNyXlBEO1ZbIoF\\", \\"timestamp\\": \\"2024-04-18T13:45:05.070108+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABMPoAd3IKOMLULV4FjmOGR89YnB4jRpI9F9onUOWiEREAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"ff69554c8bc05f99d5021ddde7ff67e24c46f125dc1e4c7da8ac88512cd1b8aa\\"}"}
f6b31925-2bb5-4771-a7a2-e8dd5b7c4a39	8fa6f334-a0e4-4542-a0a0-4b8b69421161	horizon response error: StatusCode=400, Type=https://stellar.org/horizon-errors/transaction_failed, Title=Transaction Failed, Detail=The transaction failed when submitted to the stellar network. The `extras.result_codes` field on this response contains further details.  Descriptions of each code can be found at: https://developers.stellar.org/api/errors/http-status-codes/horizon-specific/transaction-failed/, Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_failed - operation codes: [ op_no_trust ]	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GBLDUES2TP67B2MWDSLR3XEJQUHR5MEFPKCWM6M3FIP2AEOH2COMYV6N	2024-03-01 21:37:23.562068+00	\N	2024-03-01 21:37:41.524179+00	2024-03-01 21:37:44.525422+00	abbe9c4016c54e912983110fe7e2939351564483bbdefc77290435e4504b85de	1	2024-03-01 21:37:53.564799+00	2024-03-01 21:37:53.564799+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAb0LEEo+94GoNXx9zSSdObfBSH/Z8ZSaNo2mLvPt9iEwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZeJL0QAAAAEAAAAAAAYQMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABWOhJam/3w6ZYclx3ciYUPHrCFeoVmeZsqH6ARx9CczAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXMm18f+1YlxXIICq7LtCHzZWSXMT9SnjMVBnSbv0GK1bBV2ot4ZfrZr0anl4mAAc8goaidpp4L6m0xKN9Uq6CD7fYhMAAABAmrXGanMX5o/tRnu/mj+RZuY4gHMNDC4jmZ+o9AS6NZk1j3qnyU7/HF5vmeSNFxSY3p+h9jxvu0gvdB/DrfPjCwAAAAAAAAAB7OKZaQAAAECR6a3OJq1Wj07ot08ZpYY6h/lHo6uEqkQHCSRbVDvC7BXPhZCYOWBt6LHmi2q7zhkEyjpM9p0e+YSOPUFXO9MJ	\N	\N	\N	ERROR	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-03-01T21:37:23.562068+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAb0LEEo+94GoNXx9zSSdObfBSH/Z8ZSaNo2mLvPt9iEwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZeJL0QAAAAEAAAAAAAYQMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABWOhJam/3w6ZYclx3ciYUPHrCFeoVmeZsqH6ARx9CczAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXMm18f+1YlxXIICq7LtCHzZWSXMT9SnjMVBnSbv0GK1bBV2ot4ZfrZr0anl4mAAc8goaidpp4L6m0xKN9Uq6CD7fYhMAAABAmrXGanMX5o/tRnu/mj+RZuY4gHMNDC4jmZ+o9AS6NZk1j3qnyU7/HF5vmeSNFxSY3p+h9jxvu0gvdB/DrfPjCwAAAAAAAAAB7OKZaQAAAECR6a3OJq1Wj07ot08ZpYY6h/lHo6uEqkQHCSRbVDvC7BXPhZCYOWBt6LHmi2q7zhkEyjpM9p0e+YSOPUFXO9MJ\\", \\"timestamp\\": \\"2024-03-01T21:37:41.524179+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"abbe9c4016c54e912983110fe7e2939351564483bbdefc77290435e4504b85de\\"}","{\\"status\\": \\"ERROR\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAb0LEEo+94GoNXx9zSSdObfBSH/Z8ZSaNo2mLvPt9iEwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZeJL0QAAAAEAAAAAAAYQMwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABWOhJam/3w6ZYclx3ciYUPHrCFeoVmeZsqH6ARx9CczAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXMm18f+1YlxXIICq7LtCHzZWSXMT9SnjMVBnSbv0GK1bBV2ot4ZfrZr0anl4mAAc8goaidpp4L6m0xKN9Uq6CD7fYhMAAABAmrXGanMX5o/tRnu/mj+RZuY4gHMNDC4jmZ+o9AS6NZk1j3qnyU7/HF5vmeSNFxSY3p+h9jxvu0gvdB/DrfPjCwAAAAAAAAAB7OKZaQAAAECR6a3OJq1Wj07ot08ZpYY6h/lHo6uEqkQHCSRbVDvC7BXPhZCYOWBt6LHmi2q7zhkEyjpM9p0e+YSOPUFXO9MJ\\", \\"timestamp\\": \\"2024-03-01T21:37:44.525422+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"horizon response error: StatusCode=400, Type=https://stellar.org/horizon-errors/transaction_failed, Title=Transaction Failed, Detail=The transaction failed when submitted to the stellar network. The `extras.result_codes` field on this response contains further details.  Descriptions of each code can be found at: https://developers.stellar.org/api/errors/http-status-codes/horizon-specific/transaction-failed/, Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_failed - operation codes: [ op_no_trust ]\\", \\"stellar_transaction_hash\\": \\"abbe9c4016c54e912983110fe7e2939351564483bbdefc77290435e4504b85de\\"}"}
3217f528-2d2d-47c0-ba57-dc28c1bc1176	9294a461-5277-4bd5-9c73-74efed2192c9	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-04-25 17:17:35.176125+00	\N	2024-04-25 17:17:49.636629+00	2024-04-25 17:17:54.791597+00	21930d96cc4a33b113d57de72f602c7f45eb156dc24ad29ecbf87f1fddde14ed	1	2024-04-25 17:17:55.175895+00	2024-04-25 17:17:55.175895+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZiqRaQAAAAEAAAAAABPQvgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABCMp5ccH7pP0pZW+uKroucUhbhUGniBQA8fsW6VBiUEnENbUdJJg+9/1f2T+NRWXG1IiGciA65oNKL5w6LoBAs/WmRgAAABA7Va2IbQKM+Ya8pdMjwmMQfL7oBB0m7+hIuOqHU2LTAUMb/5GbT5FVzIDuErZ/8jFclx2DHYt0i3Ry96+c3weBQAAAAAAAAAB7OKZaQAAAEBl0O+QEGMFbzQjerbf5s9f1RhWdHz0zYZaM9TZ+97ZMkT4Cqchlgt/7yJ6r8rLgLZVf45gaZo1f1Ue9358jO8L	AAAAAAAAAMgAAAABz6mVKpoZRD4PiniWSo8AfXeHQsaF8rrznDD5ftBFDwcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-04-25T17:17:35.176125+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZiqRaQAAAAEAAAAAABPQvgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABCMp5ccH7pP0pZW+uKroucUhbhUGniBQA8fsW6VBiUEnENbUdJJg+9/1f2T+NRWXG1IiGciA65oNKL5w6LoBAs/WmRgAAABA7Va2IbQKM+Ya8pdMjwmMQfL7oBB0m7+hIuOqHU2LTAUMb/5GbT5FVzIDuErZ/8jFclx2DHYt0i3Ry96+c3weBQAAAAAAAAAB7OKZaQAAAEBl0O+QEGMFbzQjerbf5s9f1RhWdHz0zYZaM9TZ+97ZMkT4Cqchlgt/7yJ6r8rLgLZVf45gaZo1f1Ue9358jO8L\\", \\"timestamp\\": \\"2024-04-25T17:17:49.636629+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"21930d96cc4a33b113d57de72f602c7f45eb156dc24ad29ecbf87f1fddde14ed\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZiqRaQAAAAEAAAAAABPQvgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABCMp5ccH7pP0pZW+uKroucUhbhUGniBQA8fsW6VBiUEnENbUdJJg+9/1f2T+NRWXG1IiGciA65oNKL5w6LoBAs/WmRgAAABA7Va2IbQKM+Ya8pdMjwmMQfL7oBB0m7+hIuOqHU2LTAUMb/5GbT5FVzIDuErZ/8jFclx2DHYt0i3Ry96+c3weBQAAAAAAAAAB7OKZaQAAAEBl0O+QEGMFbzQjerbf5s9f1RhWdHz0zYZaM9TZ+97ZMkT4Cqchlgt/7yJ6r8rLgLZVf45gaZo1f1Ue9358jO8L\\", \\"timestamp\\": \\"2024-04-25T17:17:54.788951+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABz6mVKpoZRD4PiniWSo8AfXeHQsaF8rrznDD5ftBFDwcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"21930d96cc4a33b113d57de72f602c7f45eb156dc24ad29ecbf87f1fddde14ed\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAAUpLy0VzB+Rsc9VGJiE4nxha9upU3XLTtjAOC6z9aZGAABhqAABEziAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZiqRaQAAAAEAAAAAABPQvgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABABCMp5ccH7pP0pZW+uKroucUhbhUGniBQA8fsW6VBiUEnENbUdJJg+9/1f2T+NRWXG1IiGciA65oNKL5w6LoBAs/WmRgAAABA7Va2IbQKM+Ya8pdMjwmMQfL7oBB0m7+hIuOqHU2LTAUMb/5GbT5FVzIDuErZ/8jFclx2DHYt0i3Ry96+c3weBQAAAAAAAAAB7OKZaQAAAEBl0O+QEGMFbzQjerbf5s9f1RhWdHz0zYZaM9TZ+97ZMkT4Cqchlgt/7yJ6r8rLgLZVf45gaZo1f1Ue9358jO8L\\", \\"timestamp\\": \\"2024-04-25T17:17:54.791597+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABz6mVKpoZRD4PiniWSo8AfXeHQsaF8rrznDD5ftBFDwcAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"21930d96cc4a33b113d57de72f602c7f45eb156dc24ad29ecbf87f1fddde14ed\\"}"}
caa71aab-643c-447e-89d6-752553c1677f	7483b195-0cbf-482e-9d41-05df186b75b5	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GARGMZ6CX25KPH3ZZEK4ZRN63SEDRHTZDKXL27UYX54JBITHXXIXJTBN	2024-06-11 14:48:30.514827+00	\N	2024-06-11 14:48:34.056707+00	2024-06-11 14:48:38.126167+00	ce864b259ff9e086c3d85104328128b459f14e6a665d9731e295ac7e731415e1	1	2024-06-11 14:48:40.51209+00	2024-06-11 14:48:40.51209+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADXifQopEJ2MNaY8dDVeA3vnUm1OhCDAwYSiTJqobECRAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhk7gAAAAEAAAAAAB+UdAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAiZmfCvrqnn3nJFczFvtyIOJ55Gq69fpi/eJCiZ73RdAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAO0e/pLyS8k93pMiaUe9C7HdC9k85kwa8dgylj/K3cSSS7vH3ActJiJoZRrhnxalrApZqy7H3BY502ggyDbN+BaGxAkQAAABATiF8U7+LIAes9+aTVOeCD/k+u/e5datOJTtODXyjaBhYmHAWPu6CG6x66zJsBU4Vk7QJ1l0rUATQYHFo8XGoBAAAAAAAAAAB7OKZaQAAAECbFfEXj4iL+9aHXupAt3gvhnSL2kFvl0szAMtqkq/XHoKtiD2YogqsGF5+kStCtJZx97yrXTlscg/uq7IqhKEM	AAAAAAAAAMgAAAABaip1feYIED61tt4VHbqg2o1JTD997imvQ2XnSQ6dCN0AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-11T14:48:30.514827+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADXifQopEJ2MNaY8dDVeA3vnUm1OhCDAwYSiTJqobECRAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhk7gAAAAEAAAAAAB+UdAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAiZmfCvrqnn3nJFczFvtyIOJ55Gq69fpi/eJCiZ73RdAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAO0e/pLyS8k93pMiaUe9C7HdC9k85kwa8dgylj/K3cSSS7vH3ActJiJoZRrhnxalrApZqy7H3BY502ggyDbN+BaGxAkQAAABATiF8U7+LIAes9+aTVOeCD/k+u/e5datOJTtODXyjaBhYmHAWPu6CG6x66zJsBU4Vk7QJ1l0rUATQYHFo8XGoBAAAAAAAAAAB7OKZaQAAAECbFfEXj4iL+9aHXupAt3gvhnSL2kFvl0szAMtqkq/XHoKtiD2YogqsGF5+kStCtJZx97yrXTlscg/uq7IqhKEM\\", \\"timestamp\\": \\"2024-06-11T14:48:34.056707+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"ce864b259ff9e086c3d85104328128b459f14e6a665d9731e295ac7e731415e1\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADXifQopEJ2MNaY8dDVeA3vnUm1OhCDAwYSiTJqobECRAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhk7gAAAAEAAAAAAB+UdAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAiZmfCvrqnn3nJFczFvtyIOJ55Gq69fpi/eJCiZ73RdAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAO0e/pLyS8k93pMiaUe9C7HdC9k85kwa8dgylj/K3cSSS7vH3ActJiJoZRrhnxalrApZqy7H3BY502ggyDbN+BaGxAkQAAABATiF8U7+LIAes9+aTVOeCD/k+u/e5datOJTtODXyjaBhYmHAWPu6CG6x66zJsBU4Vk7QJ1l0rUATQYHFo8XGoBAAAAAAAAAAB7OKZaQAAAECbFfEXj4iL+9aHXupAt3gvhnSL2kFvl0szAMtqkq/XHoKtiD2YogqsGF5+kStCtJZx97yrXTlscg/uq7IqhKEM\\", \\"timestamp\\": \\"2024-06-11T14:48:38.124441+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABaip1feYIED61tt4VHbqg2o1JTD997imvQ2XnSQ6dCN0AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"ce864b259ff9e086c3d85104328128b459f14e6a665d9731e295ac7e731415e1\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADXifQopEJ2MNaY8dDVeA3vnUm1OhCDAwYSiTJqobECRAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhk7gAAAAEAAAAAAB+UdAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAiZmfCvrqnn3nJFczFvtyIOJ55Gq69fpi/eJCiZ73RdAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAO0e/pLyS8k93pMiaUe9C7HdC9k85kwa8dgylj/K3cSSS7vH3ActJiJoZRrhnxalrApZqy7H3BY502ggyDbN+BaGxAkQAAABATiF8U7+LIAes9+aTVOeCD/k+u/e5datOJTtODXyjaBhYmHAWPu6CG6x66zJsBU4Vk7QJ1l0rUATQYHFo8XGoBAAAAAAAAAAB7OKZaQAAAECbFfEXj4iL+9aHXupAt3gvhnSL2kFvl0szAMtqkq/XHoKtiD2YogqsGF5+kStCtJZx97yrXTlscg/uq7IqhKEM\\", \\"timestamp\\": \\"2024-06-11T14:48:38.126167+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABaip1feYIED61tt4VHbqg2o1JTD997imvQ2XnSQ6dCN0AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"ce864b259ff9e086c3d85104328128b459f14e6a665d9731e295ac7e731415e1\\"}"}
a51b05a8-3fa2-482e-b0a6-0cd1e9704f55	144b389b-d2e4-42eb-beb7-93d602868864	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-04-17 21:36:34.656142+00	\N	2024-04-17 21:36:42.597066+00	2024-04-17 21:36:49.067407+00	4b4a73d0852b6023c1c6d3478fd0e422e548664ca7f4e273154d6e0839ec8175	1	2024-04-17 21:36:54.660121+00	2024-04-17 21:36:54.660121+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADFe9bhyGIiRRmIy5eizU2QBqrGuamyReJorXSS+yQm+QABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiBCFgAAAAEAAAAAABHaUgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1FKE/utQEhgk/mOgrCf3NBJNggth5p2KEl5kCd+LqC08a7asy7Wtro67/PgdqVXoBBZUF9LQF0kz4GyV1f8vAvskJvkAAABAOlg8pjP9M14Uxd9PlCDwpkH0HMvzjZfUgH+bvUgK+hSig2v2U2pN2WV3Oh0wKO3B4CNBFmtK3L/wIvQ2iHuaBgAAAAAAAAAB7OKZaQAAAEDBGrlq3fRhVxBLOqaCMvSDpjERGuuKePpcGPZXV1AwX0+nrf9fcL/411l2h37XP/nCDJ2lSpAxmf9Bz6Y3QpAK	AAAAAAAAAMgAAAABclEN+Fz+abvYfW9VFGZ52KUHtP340dPhc/eeixNGNd4AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-04-17T21:36:34.656142+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADFe9bhyGIiRRmIy5eizU2QBqrGuamyReJorXSS+yQm+QABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiBCFgAAAAEAAAAAABHaUgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1FKE/utQEhgk/mOgrCf3NBJNggth5p2KEl5kCd+LqC08a7asy7Wtro67/PgdqVXoBBZUF9LQF0kz4GyV1f8vAvskJvkAAABAOlg8pjP9M14Uxd9PlCDwpkH0HMvzjZfUgH+bvUgK+hSig2v2U2pN2WV3Oh0wKO3B4CNBFmtK3L/wIvQ2iHuaBgAAAAAAAAAB7OKZaQAAAEDBGrlq3fRhVxBLOqaCMvSDpjERGuuKePpcGPZXV1AwX0+nrf9fcL/411l2h37XP/nCDJ2lSpAxmf9Bz6Y3QpAK\\", \\"timestamp\\": \\"2024-04-17T21:36:42.597066+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"4b4a73d0852b6023c1c6d3478fd0e422e548664ca7f4e273154d6e0839ec8175\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADFe9bhyGIiRRmIy5eizU2QBqrGuamyReJorXSS+yQm+QABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiBCFgAAAAEAAAAAABHaUgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1FKE/utQEhgk/mOgrCf3NBJNggth5p2KEl5kCd+LqC08a7asy7Wtro67/PgdqVXoBBZUF9LQF0kz4GyV1f8vAvskJvkAAABAOlg8pjP9M14Uxd9PlCDwpkH0HMvzjZfUgH+bvUgK+hSig2v2U2pN2WV3Oh0wKO3B4CNBFmtK3L/wIvQ2iHuaBgAAAAAAAAAB7OKZaQAAAEDBGrlq3fRhVxBLOqaCMvSDpjERGuuKePpcGPZXV1AwX0+nrf9fcL/411l2h37XP/nCDJ2lSpAxmf9Bz6Y3QpAK\\", \\"timestamp\\": \\"2024-04-17T21:36:49.065329+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABclEN+Fz+abvYfW9VFGZ52KUHtP340dPhc/eeixNGNd4AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"4b4a73d0852b6023c1c6d3478fd0e422e548664ca7f4e273154d6e0839ec8175\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADFe9bhyGIiRRmIy5eizU2QBqrGuamyReJorXSS+yQm+QABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZiBCFgAAAAEAAAAAABHaUgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABA1FKE/utQEhgk/mOgrCf3NBJNggth5p2KEl5kCd+LqC08a7asy7Wtro67/PgdqVXoBBZUF9LQF0kz4GyV1f8vAvskJvkAAABAOlg8pjP9M14Uxd9PlCDwpkH0HMvzjZfUgH+bvUgK+hSig2v2U2pN2WV3Oh0wKO3B4CNBFmtK3L/wIvQ2iHuaBgAAAAAAAAAB7OKZaQAAAEDBGrlq3fRhVxBLOqaCMvSDpjERGuuKePpcGPZXV1AwX0+nrf9fcL/411l2h37XP/nCDJ2lSpAxmf9Bz6Y3QpAK\\", \\"timestamp\\": \\"2024-04-17T21:36:49.067407+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABclEN+Fz+abvYfW9VFGZ52KUHtP340dPhc/eeixNGNd4AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"4b4a73d0852b6023c1c6d3478fd0e422e548664ca7f4e273154d6e0839ec8175\\"}"}
401375ca-533e-4a0f-b7b7-c78922533810	78bf1822-aeaa-4797-95bf-bf1890d23c42	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDJUCRN3DXK2JXDS7SVII2UDACR5L2LQUOBLO5H6PYIIK62J37TRVFNN	2024-03-07 17:02:53.780316+00	\N	2024-03-07 17:03:08.968624+00	2024-03-07 17:03:12.545345+00	d0316c750af02c639f069efc816c0cac1cf0d41c95b326d86470780e1fbd14bc	1	2024-03-07 17:03:13.781017+00	2024-03-07 17:03:13.781017+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACphCFW/piUVrsWfdg3ttUsQsqGxNL3N0Df0VXT5J3BwwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZen0eAAAAAEAAAAAAAeFBAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAk3i3y/MBCGtE5QchKE6rDm64lt/QPpDEOWM7h54NPAsAdfvn7Fu5vEno+jN13NPOoBBAEv4wgoajaLaglfi1AOSdwcMAAABASAnGX1db5ygclR2XaiUG0lAdx9qBNzZusSMhKUTSpYYu5bKFZDz2Pi+xlKh+rUgI18RDezPjvj9wA+7RhchaAQAAAAAAAAAB7OKZaQAAAEBroXo99VDARl0BVb7j2EbhobcFHKfh0gzv3reA5j7uQ/aTLo9xo9U5eEGh9fIFO0l/sTUJHdRQ51DYMfIbwz4D	AAAAAAAAAMgAAAABK40toYsrg1WmrDl2yS9ZJFmyjkZ5i2NnVwZAe6AzVQ8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-03-07T17:02:53.780316+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACphCFW/piUVrsWfdg3ttUsQsqGxNL3N0Df0VXT5J3BwwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZen0eAAAAAEAAAAAAAeFBAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAk3i3y/MBCGtE5QchKE6rDm64lt/QPpDEOWM7h54NPAsAdfvn7Fu5vEno+jN13NPOoBBAEv4wgoajaLaglfi1AOSdwcMAAABASAnGX1db5ygclR2XaiUG0lAdx9qBNzZusSMhKUTSpYYu5bKFZDz2Pi+xlKh+rUgI18RDezPjvj9wA+7RhchaAQAAAAAAAAAB7OKZaQAAAEBroXo99VDARl0BVb7j2EbhobcFHKfh0gzv3reA5j7uQ/aTLo9xo9U5eEGh9fIFO0l/sTUJHdRQ51DYMfIbwz4D\\", \\"timestamp\\": \\"2024-03-07T17:03:08.968624+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"d0316c750af02c639f069efc816c0cac1cf0d41c95b326d86470780e1fbd14bc\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACphCFW/piUVrsWfdg3ttUsQsqGxNL3N0Df0VXT5J3BwwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZen0eAAAAAEAAAAAAAeFBAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAk3i3y/MBCGtE5QchKE6rDm64lt/QPpDEOWM7h54NPAsAdfvn7Fu5vEno+jN13NPOoBBAEv4wgoajaLaglfi1AOSdwcMAAABASAnGX1db5ygclR2XaiUG0lAdx9qBNzZusSMhKUTSpYYu5bKFZDz2Pi+xlKh+rUgI18RDezPjvj9wA+7RhchaAQAAAAAAAAAB7OKZaQAAAEBroXo99VDARl0BVb7j2EbhobcFHKfh0gzv3reA5j7uQ/aTLo9xo9U5eEGh9fIFO0l/sTUJHdRQ51DYMfIbwz4D\\", \\"timestamp\\": \\"2024-03-07T17:03:12.537403+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABK40toYsrg1WmrDl2yS9ZJFmyjkZ5i2NnVwZAe6AzVQ8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"d0316c750af02c639f069efc816c0cac1cf0d41c95b326d86470780e1fbd14bc\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACphCFW/piUVrsWfdg3ttUsQsqGxNL3N0Df0VXT5J3BwwABhqAABEziAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZen0eAAAAAEAAAAAAAeFBAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADTQUW7HdWk3HL8qoRqgwCj1elwo4K3dP5+EIV7Sd/nGgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAk3i3y/MBCGtE5QchKE6rDm64lt/QPpDEOWM7h54NPAsAdfvn7Fu5vEno+jN13NPOoBBAEv4wgoajaLaglfi1AOSdwcMAAABASAnGX1db5ygclR2XaiUG0lAdx9qBNzZusSMhKUTSpYYu5bKFZDz2Pi+xlKh+rUgI18RDezPjvj9wA+7RhchaAQAAAAAAAAAB7OKZaQAAAEBroXo99VDARl0BVb7j2EbhobcFHKfh0gzv3reA5j7uQ/aTLo9xo9U5eEGh9fIFO0l/sTUJHdRQ51DYMfIbwz4D\\", \\"timestamp\\": \\"2024-03-07T17:03:12.545345+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABK40toYsrg1WmrDl2yS9ZJFmyjkZ5i2NnVwZAe6AzVQ8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"d0316c750af02c639f069efc816c0cac1cf0d41c95b326d86470780e1fbd14bc\\"}"}
ca101403-934f-4aa2-af9f-e6f0f4a94774	31a26bef-6261-42f6-bfac-00c63b37c73f	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	50.0000000	GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM7BNY2F	2024-05-01 14:01:12.517849+00	\N	2024-05-01 14:01:12.840692+00	2024-05-01 14:01:13.678721+00	b00c4ae10c7a57c48f80b7f0badf9b1c0d3104b5167360762d722f654f75f13e	1	2024-05-01 14:01:22.518588+00	2024-05-01 14:01:22.518588+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACjuceo3QHUNHgodwSafznZMVsr5nCCYMdfrpWTD4CdZAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJMVAAAAAEAAAAAABVJUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAuYlxKeg01m5n1DKRzwhFRbeLz55f3BtXiIQaZ2fEKMIt3iXbmIMhm7L+EK1PVB3xEsSc1cSkPzLbqdiU6Bv3Cw+AnWQAAABA7/XgQ42ocCfwX/aei08ZLQaXyx/OOlZ6DbTqbNmaoaSuJHRjRso9xv/qRkmcSQRwocivZBc+ZEa7FFAuAXhNCwAAAAAAAAAB7OKZaQAAAEBO/3D6V8syTThg8gX87S1v2owRX2B4xioxk6zAwV1uaBwqHIxd5yfiVgWQJfpwj1gz3c5EUdSbcBFHBSjT2o4D	AAAAAAAAAMgAAAABNpmsx+5fxLtwkG45s8KROwjMxT8HLFPDsA4S2NNrWNEAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-05-01T14:01:12.517849+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACjuceo3QHUNHgodwSafznZMVsr5nCCYMdfrpWTD4CdZAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJMVAAAAAEAAAAAABVJUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAuYlxKeg01m5n1DKRzwhFRbeLz55f3BtXiIQaZ2fEKMIt3iXbmIMhm7L+EK1PVB3xEsSc1cSkPzLbqdiU6Bv3Cw+AnWQAAABA7/XgQ42ocCfwX/aei08ZLQaXyx/OOlZ6DbTqbNmaoaSuJHRjRso9xv/qRkmcSQRwocivZBc+ZEa7FFAuAXhNCwAAAAAAAAAB7OKZaQAAAEBO/3D6V8syTThg8gX87S1v2owRX2B4xioxk6zAwV1uaBwqHIxd5yfiVgWQJfpwj1gz3c5EUdSbcBFHBSjT2o4D\\", \\"timestamp\\": \\"2024-05-01T14:01:12.840692+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"b00c4ae10c7a57c48f80b7f0badf9b1c0d3104b5167360762d722f654f75f13e\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACjuceo3QHUNHgodwSafznZMVsr5nCCYMdfrpWTD4CdZAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJMVAAAAAEAAAAAABVJUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAuYlxKeg01m5n1DKRzwhFRbeLz55f3BtXiIQaZ2fEKMIt3iXbmIMhm7L+EK1PVB3xEsSc1cSkPzLbqdiU6Bv3Cw+AnWQAAABA7/XgQ42ocCfwX/aei08ZLQaXyx/OOlZ6DbTqbNmaoaSuJHRjRso9xv/qRkmcSQRwocivZBc+ZEa7FFAuAXhNCwAAAAAAAAAB7OKZaQAAAEBO/3D6V8syTThg8gX87S1v2owRX2B4xioxk6zAwV1uaBwqHIxd5yfiVgWQJfpwj1gz3c5EUdSbcBFHBSjT2o4D\\", \\"timestamp\\": \\"2024-05-01T14:01:13.676477+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABNpmsx+5fxLtwkG45s8KROwjMxT8HLFPDsA4S2NNrWNEAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"b00c4ae10c7a57c48f80b7f0badf9b1c0d3104b5167360762d722f654f75f13e\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACjuceo3QHUNHgodwSafznZMVsr5nCCYMdfrpWTD4CdZAABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZjJMVAAAAAEAAAAAABVJUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADk887pQwH/azc73x7CO8g8Kf2bhA+AFoNAQ9Alehs+FgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAB3NZQAAAAAAAAAAAuzimWkAAABAuYlxKeg01m5n1DKRzwhFRbeLz55f3BtXiIQaZ2fEKMIt3iXbmIMhm7L+EK1PVB3xEsSc1cSkPzLbqdiU6Bv3Cw+AnWQAAABA7/XgQ42ocCfwX/aei08ZLQaXyx/OOlZ6DbTqbNmaoaSuJHRjRso9xv/qRkmcSQRwocivZBc+ZEa7FFAuAXhNCwAAAAAAAAAB7OKZaQAAAEBO/3D6V8syTThg8gX87S1v2owRX2B4xioxk6zAwV1uaBwqHIxd5yfiVgWQJfpwj1gz3c5EUdSbcBFHBSjT2o4D\\", \\"timestamp\\": \\"2024-05-01T14:01:13.678721+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABNpmsx+5fxLtwkG45s8KROwjMxT8HLFPDsA4S2NNrWNEAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"b00c4ae10c7a57c48f80b7f0badf9b1c0d3104b5167360762d722f654f75f13e\\"}"}
42542da9-dae3-40a8-98b8-42f3ee446585	1277b3c5-fa16-4ea5-a789-e738d9bfc5f6	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GAOFCIOXJT3OSZI2IPVN5RKYPSUKM3AZDFHJ4657D3QR74UAZ4OZHZD2	2024-06-07 04:36:10.527436+00	\N	2024-06-07 04:36:14.058428+00	2024-06-07 04:36:16.79112+00	f3a96dc70a58435bcc6f796f9b74622ba802c4f7ef2ce37bef712094ad86a773	1	2024-06-07 04:36:20.526156+00	2024-06-07 04:36:20.526156+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACEoM6T2yr3fNFMfwxlO4sI5KGKPRvbnB1VJU5eeMJE0AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmKPagAAAAEAAAAAAB54AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAcUSHXTPbpZRpD6t7FWHyopmwZGU6ee78e4R/ygM8dkwAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABACyOBwLqw+J2cTay4rX6W6VXmtpTFHipnJXRQfoTYN3SxVxd3JGADuI2lk635L/skMK2MeiL4e5dJVdVDJjFyBXjCRNAAAABAw6vAbeiyM8urwBFJZT08PhBcfBztr1KLrketM9KUxn9Vxb+bhol1q8JilZ77tn/iGj+TSx1iXVDAbhdaPdKJDwAAAAAAAAAB7OKZaQAAAED39SfZMrXy5FBmXSmzRMktafA9bmdN/YzFk4PQu6vy1BN14cycZZFHwaejtYSaUxCg585jjgradoRTFW69t74J	AAAAAAAAAMgAAAABRW2IBujXp38bUGlp4mqDDnFf3ojHhFzVcKnEpyZ3np8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-07T04:36:10.527436+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACEoM6T2yr3fNFMfwxlO4sI5KGKPRvbnB1VJU5eeMJE0AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmKPagAAAAEAAAAAAB54AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAcUSHXTPbpZRpD6t7FWHyopmwZGU6ee78e4R/ygM8dkwAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABACyOBwLqw+J2cTay4rX6W6VXmtpTFHipnJXRQfoTYN3SxVxd3JGADuI2lk635L/skMK2MeiL4e5dJVdVDJjFyBXjCRNAAAABAw6vAbeiyM8urwBFJZT08PhBcfBztr1KLrketM9KUxn9Vxb+bhol1q8JilZ77tn/iGj+TSx1iXVDAbhdaPdKJDwAAAAAAAAAB7OKZaQAAAED39SfZMrXy5FBmXSmzRMktafA9bmdN/YzFk4PQu6vy1BN14cycZZFHwaejtYSaUxCg585jjgradoRTFW69t74J\\", \\"timestamp\\": \\"2024-06-07T04:36:14.058428+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"f3a96dc70a58435bcc6f796f9b74622ba802c4f7ef2ce37bef712094ad86a773\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACEoM6T2yr3fNFMfwxlO4sI5KGKPRvbnB1VJU5eeMJE0AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmKPagAAAAEAAAAAAB54AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAcUSHXTPbpZRpD6t7FWHyopmwZGU6ee78e4R/ygM8dkwAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABACyOBwLqw+J2cTay4rX6W6VXmtpTFHipnJXRQfoTYN3SxVxd3JGADuI2lk635L/skMK2MeiL4e5dJVdVDJjFyBXjCRNAAAABAw6vAbeiyM8urwBFJZT08PhBcfBztr1KLrketM9KUxn9Vxb+bhol1q8JilZ77tn/iGj+TSx1iXVDAbhdaPdKJDwAAAAAAAAAB7OKZaQAAAED39SfZMrXy5FBmXSmzRMktafA9bmdN/YzFk4PQu6vy1BN14cycZZFHwaejtYSaUxCg585jjgradoRTFW69t74J\\", \\"timestamp\\": \\"2024-06-07T04:36:16.789498+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABRW2IBujXp38bUGlp4mqDDnFf3ojHhFzVcKnEpyZ3np8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"f3a96dc70a58435bcc6f796f9b74622ba802c4f7ef2ce37bef712094ad86a773\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAACEoM6T2yr3fNFMfwxlO4sI5KGKPRvbnB1VJU5eeMJE0AABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmKPagAAAAEAAAAAAB54AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAcUSHXTPbpZRpD6t7FWHyopmwZGU6ee78e4R/ygM8dkwAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABACyOBwLqw+J2cTay4rX6W6VXmtpTFHipnJXRQfoTYN3SxVxd3JGADuI2lk635L/skMK2MeiL4e5dJVdVDJjFyBXjCRNAAAABAw6vAbeiyM8urwBFJZT08PhBcfBztr1KLrketM9KUxn9Vxb+bhol1q8JilZ77tn/iGj+TSx1iXVDAbhdaPdKJDwAAAAAAAAAB7OKZaQAAAED39SfZMrXy5FBmXSmzRMktafA9bmdN/YzFk4PQu6vy1BN14cycZZFHwaejtYSaUxCg585jjgradoRTFW69t74J\\", \\"timestamp\\": \\"2024-06-07T04:36:16.79112+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABRW2IBujXp38bUGlp4mqDDnFf3ojHhFzVcKnEpyZ3np8AAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"f3a96dc70a58435bcc6f796f9b74622ba802c4f7ef2ce37bef712094ad86a773\\"}"}
759b55a6-6061-4397-ae02-86f864437931	9336b377-8922-4014-868d-9a2bad8e9ef4	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GBBMN6CHPVLHATN6B7GP2KYOCTLOGF4IML7WRL4VMIOXXJNSV5GCUDAB	2024-06-12 16:37:50.525664+00	\N	2024-06-12 16:41:54.04118+00	2024-06-12 16:41:58.368047+00	b566fa278958aeac1f457f7f13e4adc6b3037a2099b8ab4941e48c3b94e476e1	1	2024-06-12 16:42:00.528987+00	2024-06-12 16:42:00.528987+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmnQ/gAAAAEAAAAAAAA1LgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABCxvhHfVZwTb4PzP0rDhTW4xeIYv9or5ViHXulsq9MKgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIBz6UoXTsF8PauJh/fLIJi3qXC9v5qAkOhrqNTDCg5KcPma1oIwHqbkuvmYo7dVl6pCQRpq8Aw2ni+07/jpsBnbIwrAAAABAvF+g/CVF9Ja/TLaBRX+tZHrs3AmDJXIQZmIE57Uz9NdYq74uQH2/H35ygq1vGmVPCkgav5V13HuCNMdc/S0nAwAAAAAAAAAB7OKZaQAAAEDqXLpEjCTIGQOk3m1FE/2D0tmL1jzSzdjsQUDNzzShJvmLeVjwdT5sDfort00lf0MlOHHi3REMwUGbjYJxFy8M	AAAAAAAAAMgAAAABzFp5/RToERMvSYgqjl5yOUufDBRyTf2xbGbgP9Wok7MAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-12T16:37:50.525664+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmnQ/gAAAAEAAAAAAAA1LgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABCxvhHfVZwTb4PzP0rDhTW4xeIYv9or5ViHXulsq9MKgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIBz6UoXTsF8PauJh/fLIJi3qXC9v5qAkOhrqNTDCg5KcPma1oIwHqbkuvmYo7dVl6pCQRpq8Aw2ni+07/jpsBnbIwrAAAABAvF+g/CVF9Ja/TLaBRX+tZHrs3AmDJXIQZmIE57Uz9NdYq74uQH2/H35ygq1vGmVPCkgav5V13HuCNMdc/S0nAwAAAAAAAAAB7OKZaQAAAEDqXLpEjCTIGQOk3m1FE/2D0tmL1jzSzdjsQUDNzzShJvmLeVjwdT5sDfort00lf0MlOHHi3REMwUGbjYJxFy8M\\", \\"timestamp\\": \\"2024-06-12T16:41:54.04118+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"b566fa278958aeac1f457f7f13e4adc6b3037a2099b8ab4941e48c3b94e476e1\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmnQ/gAAAAEAAAAAAAA1LgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABCxvhHfVZwTb4PzP0rDhTW4xeIYv9or5ViHXulsq9MKgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIBz6UoXTsF8PauJh/fLIJi3qXC9v5qAkOhrqNTDCg5KcPma1oIwHqbkuvmYo7dVl6pCQRpq8Aw2ni+07/jpsBnbIwrAAAABAvF+g/CVF9Ja/TLaBRX+tZHrs3AmDJXIQZmIE57Uz9NdYq74uQH2/H35ygq1vGmVPCkgav5V13HuCNMdc/S0nAwAAAAAAAAAB7OKZaQAAAEDqXLpEjCTIGQOk3m1FE/2D0tmL1jzSzdjsQUDNzzShJvmLeVjwdT5sDfort00lf0MlOHHi3REMwUGbjYJxFy8M\\", \\"timestamp\\": \\"2024-06-12T16:41:58.366387+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABzFp5/RToERMvSYgqjl5yOUufDBRyTf2xbGbgP9Wok7MAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"b566fa278958aeac1f457f7f13e4adc6b3037a2099b8ab4941e48c3b94e476e1\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmnQ/gAAAAEAAAAAAAA1LgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABCxvhHfVZwTb4PzP0rDhTW4xeIYv9or5ViHXulsq9MKgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAIBz6UoXTsF8PauJh/fLIJi3qXC9v5qAkOhrqNTDCg5KcPma1oIwHqbkuvmYo7dVl6pCQRpq8Aw2ni+07/jpsBnbIwrAAAABAvF+g/CVF9Ja/TLaBRX+tZHrs3AmDJXIQZmIE57Uz9NdYq74uQH2/H35ygq1vGmVPCkgav5V13HuCNMdc/S0nAwAAAAAAAAAB7OKZaQAAAEDqXLpEjCTIGQOk3m1FE/2D0tmL1jzSzdjsQUDNzzShJvmLeVjwdT5sDfort00lf0MlOHHi3REMwUGbjYJxFy8M\\", \\"timestamp\\": \\"2024-06-12T16:41:58.368047+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABzFp5/RToERMvSYgqjl5yOUufDBRyTf2xbGbgP9Wok7MAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"b566fa278958aeac1f457f7f13e4adc6b3037a2099b8ab4941e48c3b94e476e1\\"}"}
a0358a9b-1a1c-4db4-8c2b-6ac9f51571eb	8c4cc967-45d0-44ea-b802-568408632169	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GAMK7PX4TXWUXXPPI4PG4JA4W22WJUIS7GAQVBFX6SQMZAN6FT6XE2F7	2024-06-11 00:40:40.529246+00	\N	2024-06-11 00:40:54.047183+00	2024-06-11 00:41:00.136202+00	808f1086ad35efdadc573a1b70b8dc5829cdc75ad92972b1a40bb62b28dc3ed8	1	2024-06-11 00:41:00.535688+00	2024-06-11 00:41:00.535688+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA/7GlXvR+YYEdKhvrXsAnHBQ3hbePgVfH7y1akiJ/9sgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmeeQgAAAAEAAAAAAB9ujwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAYr778ne1L3e9HHm4kHLa1ZNES+YEKhLf0oMyBviz9cgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAqDDMmbR3TCC3enpVLsSc+7uK1I2XLLp+xyPhj2rixQ8AyPXsKe8K7gfMGPNVU5itolO9TC28g8rvAtx+jLrVCIif/bIAAABAYeHwQdKUhhgmtU9cnmwH+A7xFdBGYl+V/BUMcVWWgxYUC/Wo9KTF6Mg00NwbIidZR3xkD7E+d2GmHI9KCgZkCAAAAAAAAAAB7OKZaQAAAEAcsT9Sma0CkBJWNcjdLbOBYPGWjLquqp/uMrnD13x21bnqeti3UtKhzmaCtP00OBfkV9L9VfmfsOyWSYlkEdID	AAAAAAAAAMgAAAAB+72EpJV9RNtUugaEkHH+C8eT3lNH8V6TXMaJSBLZlnYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-11T00:40:40.529246+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA/7GlXvR+YYEdKhvrXsAnHBQ3hbePgVfH7y1akiJ/9sgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmeeQgAAAAEAAAAAAB9ujwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAYr778ne1L3e9HHm4kHLa1ZNES+YEKhLf0oMyBviz9cgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAqDDMmbR3TCC3enpVLsSc+7uK1I2XLLp+xyPhj2rixQ8AyPXsKe8K7gfMGPNVU5itolO9TC28g8rvAtx+jLrVCIif/bIAAABAYeHwQdKUhhgmtU9cnmwH+A7xFdBGYl+V/BUMcVWWgxYUC/Wo9KTF6Mg00NwbIidZR3xkD7E+d2GmHI9KCgZkCAAAAAAAAAAB7OKZaQAAAEAcsT9Sma0CkBJWNcjdLbOBYPGWjLquqp/uMrnD13x21bnqeti3UtKhzmaCtP00OBfkV9L9VfmfsOyWSYlkEdID\\", \\"timestamp\\": \\"2024-06-11T00:40:54.047183+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"808f1086ad35efdadc573a1b70b8dc5829cdc75ad92972b1a40bb62b28dc3ed8\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA/7GlXvR+YYEdKhvrXsAnHBQ3hbePgVfH7y1akiJ/9sgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmeeQgAAAAEAAAAAAB9ujwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAYr778ne1L3e9HHm4kHLa1ZNES+YEKhLf0oMyBviz9cgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAqDDMmbR3TCC3enpVLsSc+7uK1I2XLLp+xyPhj2rixQ8AyPXsKe8K7gfMGPNVU5itolO9TC28g8rvAtx+jLrVCIif/bIAAABAYeHwQdKUhhgmtU9cnmwH+A7xFdBGYl+V/BUMcVWWgxYUC/Wo9KTF6Mg00NwbIidZR3xkD7E+d2GmHI9KCgZkCAAAAAAAAAAB7OKZaQAAAEAcsT9Sma0CkBJWNcjdLbOBYPGWjLquqp/uMrnD13x21bnqeti3UtKhzmaCtP00OBfkV9L9VfmfsOyWSYlkEdID\\", \\"timestamp\\": \\"2024-06-11T00:41:00.133125+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB+72EpJV9RNtUugaEkHH+C8eT3lNH8V6TXMaJSBLZlnYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"808f1086ad35efdadc573a1b70b8dc5829cdc75ad92972b1a40bb62b28dc3ed8\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAA/7GlXvR+YYEdKhvrXsAnHBQ3hbePgVfH7y1akiJ/9sgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmeeQgAAAAEAAAAAAB9ujwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAAAYr778ne1L3e9HHm4kHLa1ZNES+YEKhLf0oMyBviz9cgAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAqDDMmbR3TCC3enpVLsSc+7uK1I2XLLp+xyPhj2rixQ8AyPXsKe8K7gfMGPNVU5itolO9TC28g8rvAtx+jLrVCIif/bIAAABAYeHwQdKUhhgmtU9cnmwH+A7xFdBGYl+V/BUMcVWWgxYUC/Wo9KTF6Mg00NwbIidZR3xkD7E+d2GmHI9KCgZkCAAAAAAAAAAB7OKZaQAAAEAcsT9Sma0CkBJWNcjdLbOBYPGWjLquqp/uMrnD13x21bnqeti3UtKhzmaCtP00OBfkV9L9VfmfsOyWSYlkEdID\\", \\"timestamp\\": \\"2024-06-11T00:41:00.136202+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB+72EpJV9RNtUugaEkHH+C8eT3lNH8V6TXMaJSBLZlnYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"808f1086ad35efdadc573a1b70b8dc5829cdc75ad92972b1a40bb62b28dc3ed8\\"}"}
ecface82-d31a-42dd-a973-a68f9e1661fb	37a5c8a9-e2a1-4c50-941c-f57643896fae	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GCKUKUGG57SCBQAKZF4Q6DZ6VN5CXOYDRKOD2IEAYPCPN3XZVCFBYVGS	2024-06-25 17:27:50.513827+00	\N	2024-06-25 17:27:54.078364+00	2024-06-25 17:27:59.868827+00	7c8c8de5866dacf0a51392d74be8943a80aec0a0760a6631976b3afac2c51eeb	1	2024-06-25 17:28:00.51319+00	2024-06-25 17:28:00.51319+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZnr/RgAAAAEAAAAAAAN52gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACVRVDG7+QgwArJeQ8PPqt6K7sDipw9IIDDxPbu+aiKHAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXZTomH/vCEcBTuKKlf8kMNX6NvRUp1BzIXmDgiFfJzWGGqZpV6IaW6wHVHD1HuWXBJg6UqbYEswaRE5ZJ3s9DxjkIqoAAABA0ZmyTPiauU+XTco/JhiQUlxsyQAHCsPjLjSPHiSeNLNAhfwAuEEpo4LqS/rKO/TamrgWnI9P/ngrJ5L0QFhjBQAAAAAAAAAB7OKZaQAAAEAbxS8k+MH640oI6IIVoc8SP3ZukRIj7iAGd+AxMnZGf/pGWZME1DqKPu3/wEAhomloniv3giuNIDAz20RiMw0O	AAAAAAAAAMgAAAABer8yeZ1gsVHJltXLHzaoVJymZwFKl4uihWf399xq1AYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-25T17:27:50.513827+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZnr/RgAAAAEAAAAAAAN52gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACVRVDG7+QgwArJeQ8PPqt6K7sDipw9IIDDxPbu+aiKHAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXZTomH/vCEcBTuKKlf8kMNX6NvRUp1BzIXmDgiFfJzWGGqZpV6IaW6wHVHD1HuWXBJg6UqbYEswaRE5ZJ3s9DxjkIqoAAABA0ZmyTPiauU+XTco/JhiQUlxsyQAHCsPjLjSPHiSeNLNAhfwAuEEpo4LqS/rKO/TamrgWnI9P/ngrJ5L0QFhjBQAAAAAAAAAB7OKZaQAAAEAbxS8k+MH640oI6IIVoc8SP3ZukRIj7iAGd+AxMnZGf/pGWZME1DqKPu3/wEAhomloniv3giuNIDAz20RiMw0O\\", \\"timestamp\\": \\"2024-06-25T17:27:54.078364+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"7c8c8de5866dacf0a51392d74be8943a80aec0a0760a6631976b3afac2c51eeb\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZnr/RgAAAAEAAAAAAAN52gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACVRVDG7+QgwArJeQ8PPqt6K7sDipw9IIDDxPbu+aiKHAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXZTomH/vCEcBTuKKlf8kMNX6NvRUp1BzIXmDgiFfJzWGGqZpV6IaW6wHVHD1HuWXBJg6UqbYEswaRE5ZJ3s9DxjkIqoAAABA0ZmyTPiauU+XTco/JhiQUlxsyQAHCsPjLjSPHiSeNLNAhfwAuEEpo4LqS/rKO/TamrgWnI9P/ngrJ5L0QFhjBQAAAAAAAAAB7OKZaQAAAEAbxS8k+MH640oI6IIVoc8SP3ZukRIj7iAGd+AxMnZGf/pGWZME1DqKPu3/wEAhomloniv3giuNIDAz20RiMw0O\\", \\"timestamp\\": \\"2024-06-25T17:27:59.866284+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABer8yeZ1gsVHJltXLHzaoVJymZwFKl4uihWf399xq1AYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"7c8c8de5866dacf0a51392d74be8943a80aec0a0760a6631976b3afac2c51eeb\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZnr/RgAAAAEAAAAAAAN52gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACVRVDG7+QgwArJeQ8PPqt6K7sDipw9IIDDxPbu+aiKHAAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAXZTomH/vCEcBTuKKlf8kMNX6NvRUp1BzIXmDgiFfJzWGGqZpV6IaW6wHVHD1HuWXBJg6UqbYEswaRE5ZJ3s9DxjkIqoAAABA0ZmyTPiauU+XTco/JhiQUlxsyQAHCsPjLjSPHiSeNLNAhfwAuEEpo4LqS/rKO/TamrgWnI9P/ngrJ5L0QFhjBQAAAAAAAAAB7OKZaQAAAEAbxS8k+MH640oI6IIVoc8SP3ZukRIj7iAGd+AxMnZGf/pGWZME1DqKPu3/wEAhomloniv3giuNIDAz20RiMw0O\\", \\"timestamp\\": \\"2024-06-25T17:27:59.868827+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABer8yeZ1gsVHJltXLHzaoVJymZwFKl4uihWf399xq1AYAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"7c8c8de5866dacf0a51392d74be8943a80aec0a0760a6631976b3afac2c51eeb\\"}"}
9fca415f-5fa8-4e62-91e6-7e778bec3a2c	6f9fd75a-5950-4ef9-b885-46efe6686b1d	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GCDUPD3AIFYFQBCCMP5Q3DIVRWA5R2GCDZRAOKIXRDCVUQP4HSVTCZZ2	2024-06-11 01:04:50.515127+00	\N	2024-06-11 01:04:54.044529+00	2024-06-11 01:04:59.325496+00	e6186d245dc3181dd0e0b0b066b6c3f72ef5212fd3fcfef5a6f7102200acfe19	1	2024-06-11 01:05:00.531945+00	2024-06-11 01:05:00.531945+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADkEGFFFZNP6Hf0XHiokcVq/e7RdgujMTK68vqNiwraJgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmej4gAAAAEAAAAAAB9voAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACHR49gQXBYBEJj+w2NFY2B2OjCHmIHKReIxVpB/DyrMQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAJRojb6Dy0E7DI3B/9nBwj3/sC/HefSWlLHZx5s60kcQLXKwSFMAO/lCypyDhjo8jFiRfoCK1YkrIduid0s17AYsK2iYAAABARmiHBNOJywluwzuZe+WJTR6uX0OQswwyL4uAKf1PzcslSNnhMb0TMhoNlaNLlB89WqKv3lIZh2Wk1YAAiKtKAwAAAAAAAAAB7OKZaQAAAECDxGA1cCQE1WVfUAqiyj0F3j+t8o3z9fM/4gwxVoVyXvP4DJYmDsRHN4PK5lX9acOVqvDAQL+yiup/roeEEbAP	AAAAAAAAAMgAAAAB3dhhMj0EnaRpJMeeFqbfjh6bknd4TSdhMvnOTFYDd8wAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-11T01:04:50.515127+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADkEGFFFZNP6Hf0XHiokcVq/e7RdgujMTK68vqNiwraJgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmej4gAAAAEAAAAAAB9voAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACHR49gQXBYBEJj+w2NFY2B2OjCHmIHKReIxVpB/DyrMQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAJRojb6Dy0E7DI3B/9nBwj3/sC/HefSWlLHZx5s60kcQLXKwSFMAO/lCypyDhjo8jFiRfoCK1YkrIduid0s17AYsK2iYAAABARmiHBNOJywluwzuZe+WJTR6uX0OQswwyL4uAKf1PzcslSNnhMb0TMhoNlaNLlB89WqKv3lIZh2Wk1YAAiKtKAwAAAAAAAAAB7OKZaQAAAECDxGA1cCQE1WVfUAqiyj0F3j+t8o3z9fM/4gwxVoVyXvP4DJYmDsRHN4PK5lX9acOVqvDAQL+yiup/roeEEbAP\\", \\"timestamp\\": \\"2024-06-11T01:04:54.044529+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"e6186d245dc3181dd0e0b0b066b6c3f72ef5212fd3fcfef5a6f7102200acfe19\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADkEGFFFZNP6Hf0XHiokcVq/e7RdgujMTK68vqNiwraJgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmej4gAAAAEAAAAAAB9voAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACHR49gQXBYBEJj+w2NFY2B2OjCHmIHKReIxVpB/DyrMQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAJRojb6Dy0E7DI3B/9nBwj3/sC/HefSWlLHZx5s60kcQLXKwSFMAO/lCypyDhjo8jFiRfoCK1YkrIduid0s17AYsK2iYAAABARmiHBNOJywluwzuZe+WJTR6uX0OQswwyL4uAKf1PzcslSNnhMb0TMhoNlaNLlB89WqKv3lIZh2Wk1YAAiKtKAwAAAAAAAAAB7OKZaQAAAECDxGA1cCQE1WVfUAqiyj0F3j+t8o3z9fM/4gwxVoVyXvP4DJYmDsRHN4PK5lX9acOVqvDAQL+yiup/roeEEbAP\\", \\"timestamp\\": \\"2024-06-11T01:04:59.323729+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB3dhhMj0EnaRpJMeeFqbfjh6bknd4TSdhMvnOTFYDd8wAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"e6186d245dc3181dd0e0b0b066b6c3f72ef5212fd3fcfef5a6f7102200acfe19\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAADkEGFFFZNP6Hf0XHiokcVq/e7RdgujMTK68vqNiwraJgABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmej4gAAAAEAAAAAAB9voAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAACHR49gQXBYBEJj+w2NFY2B2OjCHmIHKReIxVpB/DyrMQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAJRojb6Dy0E7DI3B/9nBwj3/sC/HefSWlLHZx5s60kcQLXKwSFMAO/lCypyDhjo8jFiRfoCK1YkrIduid0s17AYsK2iYAAABARmiHBNOJywluwzuZe+WJTR6uX0OQswwyL4uAKf1PzcslSNnhMb0TMhoNlaNLlB89WqKv3lIZh2Wk1YAAiKtKAwAAAAAAAAAB7OKZaQAAAECDxGA1cCQE1WVfUAqiyj0F3j+t8o3z9fM/4gwxVoVyXvP4DJYmDsRHN4PK5lX9acOVqvDAQL+yiup/roeEEbAP\\", \\"timestamp\\": \\"2024-06-11T01:04:59.325496+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAAB3dhhMj0EnaRpJMeeFqbfjh6bknd4TSdhMvnOTFYDd8wAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"e6186d245dc3181dd0e0b0b066b6c3f72ef5212fd3fcfef5a6f7102200acfe19\\"}"}
b53b1213-cdfe-46b5-b167-bb7daf7b28cd	7756d2d0-43a2-49c7-af3c-efbca3813b85	horizon response error: StatusCode=400, Type=https://stellar.org/horizon-errors/transaction_failed, Title=Transaction Failed, Detail=The transaction failed when submitted to the stellar network. The `extras.result_codes` field on this response contains further details.  Descriptions of each code can be found at: https://developers.stellar.org/api/errors/http-status-codes/horizon-specific/transaction-failed/, Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_failed - operation codes: [ op_no_trust ]	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GBFPZN7JKFO6WELOQDUBP2ZHNXJEKH7KSWIBYRQUU2TLMD3RIUOYDE7I	2024-06-25 19:30:00.528598+00	\N	2024-06-25 19:30:14.059321+00	2024-06-25 19:30:18.960623+00	f9e4ff4c72701f89e3d58882ff6da355990117d5f4d783bcf1305962249e2041	1	2024-06-25 19:30:20.525717+00	2024-06-25 19:30:20.525717+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsb8gAAAAEAAAAAAAN/SQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABARpQeYoyLx1+7fdNnHFEvCC1IfaNrHBcGTm1M2/M0lGJ0bIIP38mzDYH18rlyPJrJ/seSi9BDD2PUnbFwT08WCHbIwrAAAABAbhLOjS5smu0bVbME0SGU9ErgF56jVCcFCfWbE2RLSX+6p5wnLxgTwZ7LxubsaCeIupl8N+wAzpDTFyXspCFFCAAAAAAAAAAB7OKZaQAAAEAWBDBGLNyXV6vTBM4dU3jVLmlgWzuqFdMV5nCz/Rekw2j3o5lIEpLaukDird8PR9x9SHCYWCE4BOroi9VVPjsB	\N	\N	\N	ERROR	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-25T19:30:00.528598+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsb8gAAAAEAAAAAAAN/SQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABARpQeYoyLx1+7fdNnHFEvCC1IfaNrHBcGTm1M2/M0lGJ0bIIP38mzDYH18rlyPJrJ/seSi9BDD2PUnbFwT08WCHbIwrAAAABAbhLOjS5smu0bVbME0SGU9ErgF56jVCcFCfWbE2RLSX+6p5wnLxgTwZ7LxubsaCeIupl8N+wAzpDTFyXspCFFCAAAAAAAAAAB7OKZaQAAAEAWBDBGLNyXV6vTBM4dU3jVLmlgWzuqFdMV5nCz/Rekw2j3o5lIEpLaukDird8PR9x9SHCYWCE4BOroi9VVPjsB\\", \\"timestamp\\": \\"2024-06-25T19:30:14.059321+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"f9e4ff4c72701f89e3d58882ff6da355990117d5f4d783bcf1305962249e2041\\"}","{\\"status\\": \\"ERROR\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABz6WGp5LnDLotDm0AbmSGFNME37nz5l15F+vhPdsjCsAABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsb8gAAAAEAAAAAAAN/SQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABARpQeYoyLx1+7fdNnHFEvCC1IfaNrHBcGTm1M2/M0lGJ0bIIP38mzDYH18rlyPJrJ/seSi9BDD2PUnbFwT08WCHbIwrAAAABAbhLOjS5smu0bVbME0SGU9ErgF56jVCcFCfWbE2RLSX+6p5wnLxgTwZ7LxubsaCeIupl8N+wAzpDTFyXspCFFCAAAAAAAAAAB7OKZaQAAAEAWBDBGLNyXV6vTBM4dU3jVLmlgWzuqFdMV5nCz/Rekw2j3o5lIEpLaukDird8PR9x9SHCYWCE4BOroi9VVPjsB\\", \\"timestamp\\": \\"2024-06-25T19:30:18.960623+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"horizon response error: StatusCode=400, Type=https://stellar.org/horizon-errors/transaction_failed, Title=Transaction Failed, Detail=The transaction failed when submitted to the stellar network. The `extras.result_codes` field on this response contains further details.  Descriptions of each code can be found at: https://developers.stellar.org/api/errors/http-status-codes/horizon-specific/transaction-failed/, Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_failed - operation codes: [ op_no_trust ]\\", \\"stellar_transaction_hash\\": \\"f9e4ff4c72701f89e3d58882ff6da355990117d5f4d783bcf1305962249e2041\\"}"}
928d1754-ae86-4296-b9d4-ada167971b93	93a1867d-01be-4857-b5a8-8ebbec37bd83	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GDXMT6WEUU6ZDRWLIOV2FYDAPUQHN45EW2TOHN5UZ6VVYYIFRIHZ3OYI	2024-06-11 14:14:20.525022+00	\N	2024-06-11 14:14:34.044928+00	2024-06-11 14:14:37.322793+00	df0ef7a009b326b78d1048fd61ec345d11f38b6a9cad60dfffe8ee000b169679	1	2024-06-11 14:14:40.525231+00	2024-06-11 14:14:40.525231+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAB71wvmn42iG2UcM+kxvPMI23BbXLp+pxmjaTQc57YuJwABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhc9gAAAAEAAAAAAB+S7gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADuyfrEpT2RxstDq6LgYH0gdvOktqbjt7TPq1xhBYoPnQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAjZ24tfziklfhkK8LhoMNSNIAsDSwatF7SrHFSt65sngYWjyYwI+pI7wj4t0sMWc5iOdHzlerWfYap0Hfd5idC+e2LicAAABAXWqdr/6nKEL+sb+0h4vB6Kz2Y4Old0CtJHPEzVwJuzDtviLnvbV7EB0AOpuWo8DUxNq4TWTWsx/p8nlAxuAjCAAAAAAAAAAB7OKZaQAAAEAVHsZ6w29c73sBpfA7m7Qs5zrBmyFG1yGcLvXY38v3IfJp9gX42XZW2MKFeg8jYcpQEcOHlWLtBPxo9YghthwN	AAAAAAAAAMgAAAABPpjSIJjf1peMqwOe6XacQhDvqjr4q37Cy676Aa5oaQQAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-11T14:14:20.525022+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAB71wvmn42iG2UcM+kxvPMI23BbXLp+pxmjaTQc57YuJwABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhc9gAAAAEAAAAAAB+S7gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADuyfrEpT2RxstDq6LgYH0gdvOktqbjt7TPq1xhBYoPnQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAjZ24tfziklfhkK8LhoMNSNIAsDSwatF7SrHFSt65sngYWjyYwI+pI7wj4t0sMWc5iOdHzlerWfYap0Hfd5idC+e2LicAAABAXWqdr/6nKEL+sb+0h4vB6Kz2Y4Old0CtJHPEzVwJuzDtviLnvbV7EB0AOpuWo8DUxNq4TWTWsx/p8nlAxuAjCAAAAAAAAAAB7OKZaQAAAEAVHsZ6w29c73sBpfA7m7Qs5zrBmyFG1yGcLvXY38v3IfJp9gX42XZW2MKFeg8jYcpQEcOHlWLtBPxo9YghthwN\\", \\"timestamp\\": \\"2024-06-11T14:14:34.044928+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"df0ef7a009b326b78d1048fd61ec345d11f38b6a9cad60dfffe8ee000b169679\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAB71wvmn42iG2UcM+kxvPMI23BbXLp+pxmjaTQc57YuJwABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhc9gAAAAEAAAAAAB+S7gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADuyfrEpT2RxstDq6LgYH0gdvOktqbjt7TPq1xhBYoPnQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAjZ24tfziklfhkK8LhoMNSNIAsDSwatF7SrHFSt65sngYWjyYwI+pI7wj4t0sMWc5iOdHzlerWfYap0Hfd5idC+e2LicAAABAXWqdr/6nKEL+sb+0h4vB6Kz2Y4Old0CtJHPEzVwJuzDtviLnvbV7EB0AOpuWo8DUxNq4TWTWsx/p8nlAxuAjCAAAAAAAAAAB7OKZaQAAAEAVHsZ6w29c73sBpfA7m7Qs5zrBmyFG1yGcLvXY38v3IfJp9gX42XZW2MKFeg8jYcpQEcOHlWLtBPxo9YghthwN\\", \\"timestamp\\": \\"2024-06-11T14:14:37.320773+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABPpjSIJjf1peMqwOe6XacQhDvqjr4q37Cy676Aa5oaQQAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"df0ef7a009b326b78d1048fd61ec345d11f38b6a9cad60dfffe8ee000b169679\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAAB71wvmn42iG2UcM+kxvPMI23BbXLp+pxmjaTQc57YuJwABhqAAE4mkAAAAAQAAAAIAAAABAAAAAAAAAAAAAAAAZmhc9gAAAAEAAAAAAB+S7gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAADuyfrEpT2RxstDq6LgYH0gdvOktqbjt7TPq1xhBYoPnQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAjZ24tfziklfhkK8LhoMNSNIAsDSwatF7SrHFSt65sngYWjyYwI+pI7wj4t0sMWc5iOdHzlerWfYap0Hfd5idC+e2LicAAABAXWqdr/6nKEL+sb+0h4vB6Kz2Y4Old0CtJHPEzVwJuzDtviLnvbV7EB0AOpuWo8DUxNq4TWTWsx/p8nlAxuAjCAAAAAAAAAAB7OKZaQAAAEAVHsZ6w29c73sBpfA7m7Qs5zrBmyFG1yGcLvXY38v3IfJp9gX42XZW2MKFeg8jYcpQEcOHlWLtBPxo9YghthwN\\", \\"timestamp\\": \\"2024-06-11T14:14:37.322793+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABPpjSIJjf1peMqwOe6XacQhDvqjr4q37Cy676Aa5oaQQAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"df0ef7a009b326b78d1048fd61ec345d11f38b6a9cad60dfffe8ee000b169679\\"}"}
8100e8ab-6c87-4d26-a179-358039372a7b	7756d2d0-43a2-49c7-af3c-efbca3813b85	\N	USDC	GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5	0.1000000	GBFPZN7JKFO6WELOQDUBP2ZHNXJEKH7KSWIBYRQUU2TLMD3RIUOYDE7I	2024-06-25 19:46:50.529293+00	\N	2024-06-25 19:46:54.054159+00	2024-06-25 19:46:56.866734+00	a4bf715a77d1d7df186d8585b536dc7eeff4ddecfc1ff1ecbdc4ef0dd11243aa	1	2024-06-25 19:47:00.513123+00	2024-06-25 19:47:00.513123+00	AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsf2gAAAAEAAAAAAAOACgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAZuwiyO3K7t6nqy8llLoZfypMQWrdhBgEORPCubcA2ukAMzDMXvXWAhIrhMk0X9mBpTjN/lRVagf2Um3qJvFTCBjkIqoAAABAz2Q8EI32VCXxbZZsuYmtl1tQ4V7cE4+lo7l2VzOW+Z1hu956zlOp+1w6Tn8o6VSlkc6brgvp52Kr55o4PoqMAAAAAAAAAAAB7OKZaQAAAEBKeoIcou009yEgxR5yNQbNxRjBgQZN91j1IB0rVtwbykZY57UL+m6REH4ZPJYHYGAdpQQeTU+0F4qLmpaC6YwA	AAAAAAAAAMgAAAABu0ACGeObfcNLwjbmXBat42d8OL0Ztt1+GJaEvppQKMgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=	\N	\N	SUCCESS	{"{\\"status\\": \\"PENDING\\", \\"xdr_sent\\": null, \\"timestamp\\": \\"2024-06-25T19:46:50.529293+00:00\\", \\"xdr_received\\": null, \\"status_message\\": null, \\"stellar_transaction_hash\\": null}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsf2gAAAAEAAAAAAAOACgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAZuwiyO3K7t6nqy8llLoZfypMQWrdhBgEORPCubcA2ukAMzDMXvXWAhIrhMk0X9mBpTjN/lRVagf2Um3qJvFTCBjkIqoAAABAz2Q8EI32VCXxbZZsuYmtl1tQ4V7cE4+lo7l2VzOW+Z1hu956zlOp+1w6Tn8o6VSlkc6brgvp52Kr55o4PoqMAAAAAAAAAAAB7OKZaQAAAEBKeoIcou009yEgxR5yNQbNxRjBgQZN91j1IB0rVtwbykZY57UL+m6REH4ZPJYHYGAdpQQeTU+0F4qLmpaC6YwA\\", \\"timestamp\\": \\"2024-06-25T19:46:54.054159+00:00\\", \\"xdr_received\\": null, \\"status_message\\": \\"Updating Stellar Transaction Hash\\", \\"stellar_transaction_hash\\": \\"a4bf715a77d1d7df186d8585b536dc7eeff4ddecfc1ff1ecbdc4ef0dd11243aa\\"}","{\\"status\\": \\"PROCESSING\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsf2gAAAAEAAAAAAAOACgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAZuwiyO3K7t6nqy8llLoZfypMQWrdhBgEORPCubcA2ukAMzDMXvXWAhIrhMk0X9mBpTjN/lRVagf2Um3qJvFTCBjkIqoAAABAz2Q8EI32VCXxbZZsuYmtl1tQ4V7cE4+lo7l2VzOW+Z1hu956zlOp+1w6Tn8o6VSlkc6brgvp52Kr55o4PoqMAAAAAAAAAAAB7OKZaQAAAEBKeoIcou009yEgxR5yNQbNxRjBgQZN91j1IB0rVtwbykZY57UL+m6REH4ZPJYHYGAdpQQeTU+0F4qLmpaC6YwA\\", \\"timestamp\\": \\"2024-06-25T19:46:56.864502+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABu0ACGeObfcNLwjbmXBat42d8OL0Ztt1+GJaEvppQKMgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": \\"Updating XDR Received\\", \\"stellar_transaction_hash\\": \\"a4bf715a77d1d7df186d8585b536dc7eeff4ddecfc1ff1ecbdc4ef0dd11243aa\\"}","{\\"status\\": \\"SUCCESS\\", \\"xdr_sent\\": \\"AAAABQAAAABFo9bFQVv7JOV0ahENSXRN5dp227UAndSK9qPl7OKZaQAAAAAAAw1AAAAAAgAAAABX1BQZWZZbUmiEmCDSx7GgWmBWAGE5owNLyEX7GOQiqgABhqAAADUfAAAAAgAAAAIAAAABAAAAAAAAAAAAAAAAZnsf2gAAAAEAAAAAAAOACgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAEWj1sVBW/sk5XRqEQ1JdE3l2nbbtQCd1Ir2o+Xs4plpAAAAAQAAAABK/LfpUV3rEW6A6BfrJ23SRR/qlZAcRhSmprYPcUUdgQAAAAFVU0RDAAAAAEI+fQXy7K+/7BkrIVo/G+lq7bjY5wJUq+NBPgIH3layAAAAAAAPQkAAAAAAAAAAAuzimWkAAABAZuwiyO3K7t6nqy8llLoZfypMQWrdhBgEORPCubcA2ukAMzDMXvXWAhIrhMk0X9mBpTjN/lRVagf2Um3qJvFTCBjkIqoAAABAz2Q8EI32VCXxbZZsuYmtl1tQ4V7cE4+lo7l2VzOW+Z1hu956zlOp+1w6Tn8o6VSlkc6brgvp52Kr55o4PoqMAAAAAAAAAAAB7OKZaQAAAEBKeoIcou009yEgxR5yNQbNxRjBgQZN91j1IB0rVtwbykZY57UL+m6REH4ZPJYHYGAdpQQeTU+0F4qLmpaC6YwA\\", \\"timestamp\\": \\"2024-06-25T19:46:56.866734+00:00\\", \\"xdr_received\\": \\"AAAAAAAAAMgAAAABu0ACGeObfcNLwjbmXBat42d8OL0Ztt1+GJaEvppQKMgAAAAAAAAAZAAAAAAAAAABAAAAAAAAAAEAAAAAAAAAAAAAAAA=\\", \\"status_message\\": null, \\"stellar_transaction_hash\\": \\"a4bf715a77d1d7df186d8585b536dc7eeff4ddecfc1ff1ecbdc4ef0dd11243aa\\"}"}
\.


--
-- Data for Name: wallets; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.wallets (id, name, homepage, deep_link_schema, created_at, updated_at, deleted_at, sep_10_client_domain, enabled) FROM stdin;
7a0c5a0a-33c1-42b9-a27b-d657567c2925	Vibrant Assist	https://vibrantapp.com/vibrant-assist	https://vibrantapp.com/sdp-dev	2023-06-02 17:26:12.27763+00	2024-05-16 10:39:38.858311+00	\N	api-dev.vibrantapp.com	t
79308ea6-da07-4520-9db4-1b9b390d5d7e	Demo Wallet	https://demo-wallet.stellar.org	https://demo-wallet.stellar.org	2023-06-02 17:26:12.490761+00	2024-05-16 10:39:38.858311+00	\N	demo-wallet-server.stellar.org	t
0c5faa7e-5dd1-4838-abf1-53771f3b04ae	BOSS Money	https://www.walletbyboss.com	https://www.walletbyboss.com	2023-06-02 17:26:12.45239+00	2024-03-07 17:07:41.899882+00	\N	www.walletbyboss.com	t
\.


--
-- Data for Name: wallets_assets; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.wallets_assets (wallet_id, asset_id) FROM stdin;
79308ea6-da07-4520-9db4-1b9b390d5d7e	4c62168d-b092-4073-b1c2-0e4c19377188
79308ea6-da07-4520-9db4-1b9b390d5d7e	e7cc851e-ed85-479f-a68d-8c74cadfa755
7a0c5a0a-33c1-42b9-a27b-d657567c2925	4c62168d-b092-4073-b1c2-0e4c19377188
\.


--
-- Name: assets assets_code_issuer_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_code_issuer_key UNIQUE (code, issuer);


--
-- Name: assets assets_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_pkey PRIMARY KEY (id);


--
-- Name: auth_migrations auth_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_migrations
    ADD CONSTRAINT auth_migrations_pkey PRIMARY KEY (id);


--
-- Name: auth_user_mfa_codes auth_user_mfa_codes_device_id_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_user_mfa_codes
    ADD CONSTRAINT auth_user_mfa_codes_device_id_code_key UNIQUE (device_id, code);


--
-- Name: auth_user_mfa_codes auth_user_mfa_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_user_mfa_codes
    ADD CONSTRAINT auth_user_mfa_codes_pkey PRIMARY KEY (device_id, auth_user_id);


--
-- Name: auth_user_password_reset auth_user_password_reset_token_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_user_password_reset
    ADD CONSTRAINT auth_user_password_reset_token_key UNIQUE (token);


--
-- Name: auth_users auth_users_email_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_users
    ADD CONSTRAINT auth_users_email_key UNIQUE (email);


--
-- Name: auth_users auth_users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_users
    ADD CONSTRAINT auth_users_pkey PRIMARY KEY (id);


--
-- Name: channel_accounts channel_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.channel_accounts
    ADD CONSTRAINT channel_accounts_pkey PRIMARY KEY (public_key);


--
-- Name: countries countries_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.countries
    ADD CONSTRAINT countries_name_key UNIQUE (name);


--
-- Name: countries countries_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.countries
    ADD CONSTRAINT countries_pkey PRIMARY KEY (code);


--
-- Name: disbursements disbursement_name_unique; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.disbursements
    ADD CONSTRAINT disbursement_name_unique UNIQUE (name);


--
-- Name: gorp_migrations gorp_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.gorp_migrations
    ADD CONSTRAINT gorp_migrations_pkey PRIMARY KEY (id);


--
-- Name: messages messages_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_pkey PRIMARY KEY (id);


--
-- Name: organizations organizations_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_name_key UNIQUE (name);


--
-- Name: organizations organizations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);


--
-- Name: receivers payments_account_phone_number_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receivers
    ADD CONSTRAINT payments_account_phone_number_key UNIQUE (phone_number);


--
-- Name: receivers payments_account_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receivers
    ADD CONSTRAINT payments_account_pkey PRIMARY KEY (id);


--
-- Name: disbursements payments_disbursement_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.disbursements
    ADD CONSTRAINT payments_disbursement_pkey PRIMARY KEY (id);


--
-- Name: payments payments_payment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_payment_pkey PRIMARY KEY (id);


--
-- Name: receiver_verifications receiver_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_verifications
    ADD CONSTRAINT receiver_verifications_pkey PRIMARY KEY (receiver_id, verification_field);


--
-- Name: receiver_wallets receiver_wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_wallets
    ADD CONSTRAINT receiver_wallets_pkey PRIMARY KEY (id);


--
-- Name: receiver_wallets receiver_wallets_receiver_id_wallet_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_wallets
    ADD CONSTRAINT receiver_wallets_receiver_id_wallet_id_key UNIQUE (receiver_id, wallet_id);


--
-- Name: submitter_transactions submitter_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.submitter_transactions
    ADD CONSTRAINT submitter_transactions_pkey PRIMARY KEY (id);


--
-- Name: submitter_transactions submitter_transactions_xdr_received_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.submitter_transactions
    ADD CONSTRAINT submitter_transactions_xdr_received_key UNIQUE (xdr_received);


--
-- Name: submitter_transactions submitter_transactions_xdr_sent_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.submitter_transactions
    ADD CONSTRAINT submitter_transactions_xdr_sent_key UNIQUE (xdr_sent);


--
-- Name: submitter_transactions unique_stellar_transaction_hash; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.submitter_transactions
    ADD CONSTRAINT unique_stellar_transaction_hash UNIQUE (stellar_transaction_hash);


--
-- Name: wallets_assets wallets_assets_wallet_id_asset_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets_assets
    ADD CONSTRAINT wallets_assets_wallet_id_asset_id_key UNIQUE (wallet_id, asset_id);


--
-- Name: wallets wallets_deep_link_schema_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets
    ADD CONSTRAINT wallets_deep_link_schema_key UNIQUE (deep_link_schema);


--
-- Name: wallets wallets_homepage_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets
    ADD CONSTRAINT wallets_homepage_key UNIQUE (homepage);


--
-- Name: wallets wallets_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets
    ADD CONSTRAINT wallets_name_key UNIQUE (name);


--
-- Name: wallets wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets
    ADD CONSTRAINT wallets_pkey PRIMARY KEY (id);


--
-- Name: disbursement_request_16523d_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX disbursement_request_16523d_idx ON public.disbursements USING btree (created_at DESC);


--
-- Name: idx_unique_external_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_unique_external_id ON public.submitter_transactions USING btree (external_id) WHERE (status <> 'ERROR'::public.transaction_status);


--
-- Name: payment_account_id_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX payment_account_id_idx ON public.payments USING btree (receiver_id);


--
-- Name: payment_account_id_like_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX payment_account_id_like_idx ON public.payments USING btree (receiver_id varchar_pattern_ops);


--
-- Name: payment_disbursement_id_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX payment_disbursement_id_idx ON public.payments USING btree (disbursement_id);


--
-- Name: payment_requested_at_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX payment_requested_at_idx ON public.payments USING btree (created_at DESC);


--
-- Name: receiver_phone_number_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX receiver_phone_number_idx ON public.receivers USING btree (phone_number varchar_pattern_ops);


--
-- Name: receiver_registered_at_idx; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX receiver_registered_at_idx ON public.receivers USING btree (created_at DESC);


--
-- Name: unique_user_valid_token; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX unique_user_valid_token ON public.auth_user_password_reset USING btree (auth_user_id, is_valid) WHERE (is_valid IS TRUE);


--
-- Name: unique_wallets_index; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX unique_wallets_index ON public.wallets USING btree (name, homepage, deep_link_schema);


--
-- Name: auth_user_mfa_codes auth_user_mfa_codes_before_update_trigger; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER auth_user_mfa_codes_before_update_trigger BEFORE UPDATE ON public.auth_user_mfa_codes FOR EACH ROW EXECUTE FUNCTION public.auth_user_mfa_codes_before_update();


--
-- Name: auth_user_password_reset auth_user_password_reset_before_insert_trigger; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER auth_user_password_reset_before_insert_trigger BEFORE INSERT ON public.auth_user_password_reset FOR EACH ROW EXECUTE FUNCTION public.auth_user_password_reset_before_insert();


--
-- Name: organizations enforce_single_row_for_organizations_delete_trigger; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER enforce_single_row_for_organizations_delete_trigger BEFORE DELETE ON public.organizations FOR EACH ROW EXECUTE FUNCTION public.enforce_single_row_for_organizations();


--
-- Name: organizations enforce_single_row_for_organizations_insert_trigger; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER enforce_single_row_for_organizations_insert_trigger BEFORE INSERT ON public.organizations FOR EACH ROW EXECUTE FUNCTION public.enforce_single_row_for_organizations();


--
-- Name: assets refresh_asset_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_asset_updated_at BEFORE UPDATE ON public.assets FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: channel_accounts refresh_channel_accounts_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_channel_accounts_updated_at BEFORE UPDATE ON public.channel_accounts FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: countries refresh_country_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_country_updated_at BEFORE UPDATE ON public.countries FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: disbursements refresh_disbursement_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_disbursement_updated_at BEFORE UPDATE ON public.disbursements FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: messages refresh_message_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_message_updated_at BEFORE UPDATE ON public.messages FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: organizations refresh_organization_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_organization_updated_at BEFORE UPDATE ON public.organizations FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: payments refresh_payment_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_payment_updated_at BEFORE UPDATE ON public.payments FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: receivers refresh_receiver_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_receiver_updated_at BEFORE UPDATE ON public.receivers FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: receiver_verifications refresh_receiver_verifications_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_receiver_verifications_updated_at BEFORE UPDATE ON public.receiver_verifications FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: receiver_wallets refresh_receiver_wallet_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_receiver_wallet_updated_at BEFORE UPDATE ON public.receiver_wallets FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: submitter_transactions refresh_submitter_transactions_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_submitter_transactions_updated_at BEFORE UPDATE ON public.submitter_transactions FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: wallets refresh_wallet_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER refresh_wallet_updated_at BEFORE UPDATE ON public.wallets FOR EACH ROW EXECUTE FUNCTION public.update_at_refresh();


--
-- Name: disbursements fk_disbursement_asset_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.disbursements
    ADD CONSTRAINT fk_disbursement_asset_id FOREIGN KEY (asset_id) REFERENCES public.assets(id);


--
-- Name: disbursements fk_disbursement_country_code; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.disbursements
    ADD CONSTRAINT fk_disbursement_country_code FOREIGN KEY (country_code) REFERENCES public.countries(code);


--
-- Name: disbursements fk_disbursement_wallet_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.disbursements
    ADD CONSTRAINT fk_disbursement_wallet_id FOREIGN KEY (wallet_id) REFERENCES public.wallets(id);


--
-- Name: auth_user_mfa_codes fk_mfa_codes_auth_user_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_user_mfa_codes
    ADD CONSTRAINT fk_mfa_codes_auth_user_id FOREIGN KEY (auth_user_id) REFERENCES public.auth_users(id);


--
-- Name: auth_user_password_reset fk_password_reset_auth_user_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.auth_user_password_reset
    ADD CONSTRAINT fk_password_reset_auth_user_id FOREIGN KEY (auth_user_id) REFERENCES public.auth_users(id);


--
-- Name: payments fk_payment_asset_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT fk_payment_asset_id FOREIGN KEY (asset_id) REFERENCES public.assets(id);


--
-- Name: payments fk_payments_receiver_wallet_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT fk_payments_receiver_wallet_id FOREIGN KEY (receiver_wallet_id) REFERENCES public.receiver_wallets(id);


--
-- Name: messages messages_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.assets(id);


--
-- Name: messages messages_receiver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_receiver_id_fkey FOREIGN KEY (receiver_id) REFERENCES public.receivers(id);


--
-- Name: messages messages_receiver_wallet_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_receiver_wallet_id_fkey FOREIGN KEY (receiver_wallet_id) REFERENCES public.receiver_wallets(id);


--
-- Name: messages messages_wallet_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.wallets(id);


--
-- Name: payments payments_payment_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_payment_account_id_fkey FOREIGN KEY (receiver_id) REFERENCES public.receivers(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: payments payments_payment_disbursement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_payment_disbursement_id_fkey FOREIGN KEY (disbursement_id) REFERENCES public.disbursements(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: receiver_verifications receiver_verifications_receiver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_verifications
    ADD CONSTRAINT receiver_verifications_receiver_id_fkey FOREIGN KEY (receiver_id) REFERENCES public.receivers(id) ON DELETE CASCADE;


--
-- Name: receiver_wallets receiver_wallets_receiver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_wallets
    ADD CONSTRAINT receiver_wallets_receiver_id_fkey FOREIGN KEY (receiver_id) REFERENCES public.receivers(id);


--
-- Name: receiver_wallets receiver_wallets_wallet_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.receiver_wallets
    ADD CONSTRAINT receiver_wallets_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.wallets(id);


--
-- Name: wallets_assets wallets_assets_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets_assets
    ADD CONSTRAINT wallets_assets_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.assets(id);


--
-- Name: wallets_assets wallets_assets_wallet_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallets_assets
    ADD CONSTRAINT wallets_assets_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.wallets(id);


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: postgres
--

GRANT ALL ON SCHEMA public TO PUBLIC;


--
-- PostgreSQL database dump complete
--

