import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import locale_en from "@/locales/en/global.json";
import locale_uk from "@/locales/uk/global.json";
import locale_es from "@/locales/es/global.json";
import locale_ru from "@/locales/ru/global.json";
import locale_ht from "@/locales/ht/global.json";
import locale_pt from "@/locales/pt/global.json";

i18next.use(initReactI18next).init({
  debug: true,
  fallbackLng: "en",
  resources: {
    en: {
      translation: locale_en,
    },
    uk: {
      translation: locale_uk,
    },
    es: {
      translation: locale_es,
    },
    ru: {
      translation: locale_ru,
    },
    ht: {
      translation: locale_ht,
    },
    pt: {
      translation: locale_pt,
    },
  },
});
