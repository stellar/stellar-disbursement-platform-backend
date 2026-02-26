import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createJSONStorage, persist } from "zustand/middleware";
import { CaptchaType, VerificationField } from "@/types/types";

// Types
export interface Store {
  jwtToken: string;
  language: string;
  org: StoreOrg;
  user: StoreUser;
  updateJwtToken: (token: string) => void;
  updateLanguage: (language: string) => void;
  updateOrg: (orgData: Partial<StoreOrg>) => void;
  updateUser: (userData: StoreUser) => void;
}

type StoreOrg = {
  privacy_policy_link: string;
  organization_name: string;
  is_registered: boolean;
  truncated_contact_info?: string;
  is_recaptcha_disabled: boolean;
  recaptcha_site_key?: string;
  captcha_type?: CaptchaType;
};

type StoreUser = {
  phone_number?: string;
  email?: string;
  verification_field?: VerificationField;
};

// Initial values
const initOrg = {
  privacy_policy_link: "",
  organization_name: "",
  is_registered: false,
  is_recaptcha_disabled: false,
};

// Store
export const createStore = () =>
  create<Store>()(
    persist(
      immer((set) => ({
        jwtToken: "",
        language: "",
        org: initOrg,
        user: {},
        updateJwtToken: (token: string) =>
          set((state) => {
            state.jwtToken = token;
          }),
        updateLanguage: (language: string) =>
          set((state) => {
            state.language = language;
          }),
        updateOrg: (orgData) =>
          set((state) => {
            state.org = { ...state.org, ...orgData };
          }),
        updateUser: (userData) =>
          set((state) => {
            state.user = { ...state.user, ...userData };
          }),
      })),
      {
        name: "sep24:store",
        storage: createJSONStorage(() => sessionStorage), // (optional). defaults to 'localStorage'
      }
    )
  );
