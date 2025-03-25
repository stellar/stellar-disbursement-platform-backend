export const LOCAL_STORAGE_WALLET_THEME = "sdp_wallet_registration_theme";
export const LOCAL_STORAGE_SAVED_LANGUAGE = "sdp_wallet_saved_language";

export enum Routes {
  BASE = "/wallet-registration",
  ROOT = "/",
  START = "/start",
  ENTER_PASSCODE = "/enter-passcode",
  SUCCESS = "/success",
}

export const SUPPORTED_LANGUAGES = ["en", "uk"];

// Env for local development
export const SDP_API_ENDPOINT = import.meta.env.DEV
  ? import.meta.env.VITE_SDP_API_ENDPOINT || ""
  : "";
