import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { VerificationField } from "@/types/types";

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
  is_recaptcha_disabled: boolean;
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
    }))
  );
