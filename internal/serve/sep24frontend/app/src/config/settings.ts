export const LOCAL_STORAGE_WALLET_THEME = "sdp_wallet_registration_theme";
export const LOCAL_STORAGE_SAVED_LANGUAGE = "sdp_wallet_saved_language";

export enum Routes {
  BASE = "/wallet-registration",
  ROOT = "/",
  START = "/start",
  VERIFY_EMAIL = "/verify/email",
  VERIFY_PHONE = "/verify/phone",
  ENTER_PASSCODE = "/enter-passcode",
}

export const SDP_API_ENDPOINT = import.meta.env.VITE_SDP_API_ENDPOINT || "";
export const SUPPORTED_LANGUAGES = ["en", "uk"];
