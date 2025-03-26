import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import locale_en from "@/locales/en/global.json";
import locale_ua from "@/locales/ua/global.json";

i18next.use(initReactI18next).init({
  debug: true,
  resources: {
    en: {
      translation: locale_en,
    },
    ua: {
      translation: locale_ua,
    },
  },
});
