// =============================================================================
// Generic
// =============================================================================
export type VerificationMethod = "email" | "phone" | null;
export type VerificationField =
  | "DATE_OF_BIRTH"
  | "YEAR_MONTH"
  | "NATIONAL_ID_NUMBER"
  | "PIN";

// =============================================================================
// API
// =============================================================================
export type ApiError = {
  error: string;
  error_code?: string;
  extras?: { [key: string]: string };
  extras_codes?: string[];
};

export type CaptchaType = "GOOGLE_RECAPTCHA_V2" | "GOOGLE_RECAPTCHA_V3";

export type Sep24DepositInitResponse = {
  privacy_policy_link: string;
  organization_name: string;
  is_registered: boolean;
  organization_logo?: string;
  truncated_contact_info?: string;
  is_recaptcha_disabled?: boolean;
  recaptcha_site_key?: string;
  captcha_type?: CaptchaType;
};

export type Sep24DepositOtpResponse = {
  message: string;
  verification_field: VerificationField;
};
