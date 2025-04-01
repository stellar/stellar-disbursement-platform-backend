import { LOCAL_STORAGE_SAVED_LANGUAGE } from "@/config/settings";

export const localStorageSavedLanguage = {
  get: () => {
    return localStorage.getItem(LOCAL_STORAGE_SAVED_LANGUAGE);
  },
  set: (language: string) => {
    return localStorage.setItem(LOCAL_STORAGE_SAVED_LANGUAGE, language);
  },
};
