import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import locale_en from "@/locales/en/global.json";
import locale_ua from "@/locales/ua/global.json";
import locale_es from "@/locales/es/global.json";
import locale_ru from "@/locales/ru/global.json";

i18next.use(initReactI18next).init({
  debug: true,
  resources: {
    en: {
      translation: locale_en,
    },
    ua: {
      translation: locale_ua,
    },
    es: {
      translation: locale_es,
    },
    ru: {
      translation: locale_ru,
    },
  },
});
