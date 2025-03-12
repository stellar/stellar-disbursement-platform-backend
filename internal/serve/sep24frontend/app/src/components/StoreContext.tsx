import { createContext } from "react";

export type StoreContextType = {
  org: StoreOrgType;
};

export type StoreOrgType = {
  privacy_policy_link: string;
  organization_name: string;
  is_registered: boolean;
  is_recaptcha_disabled: boolean;
};

export const initOrgState = {
  privacy_policy_link: "",
  organization_name: "",
  is_registered: false,
  is_recaptcha_disabled: false,
};

export const initState: StoreContextType = {
  org: initOrgState,
};

export const StoreContext = createContext<StoreContextType>(initState);
