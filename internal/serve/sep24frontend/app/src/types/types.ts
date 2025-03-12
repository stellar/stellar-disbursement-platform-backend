// =============================================================================
// Generic
// =============================================================================
export type VerificationMethod = "email" | "phone" | null;

// =============================================================================
// API
// =============================================================================
export type Sep24DepositInitResponse = {
  privacy_policy_link: string;
  organization_name: string;
  is_registered: boolean;
  organization_logo?: string;
  truncated_contact_info?: string;
  is_recaptcha_disabled?: boolean;
};
