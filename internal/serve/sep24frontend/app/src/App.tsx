import { FC, useEffect, useState } from "react";
import { BrowserRouter, Routes as RouterRoutes, Route } from "react-router-dom";
import {
  Loader,
  Notification,
  Select,
  ThemeSwitch,
} from "@stellar/design-system";
import i18next from "i18next";
import { useTranslation } from "react-i18next";

import { SelectVerificationMethod } from "@/pages/SelectVerificationMethod";
import { PasscodeEntry } from "@/pages/PasscodeEntry";
import { Success } from "@/pages/Success";
import { AlreadyRegistered } from "@/pages/AlreadyRegistered";
import { Box } from "@/components/Box";

import {
  LOCAL_STORAGE_WALLET_THEME,
  Routes,
  SUPPORTED_LANGUAGES,
} from "@/config/settings";
import { useSep24DepositInit } from "@/query/useSep24DepositInit";
import { getSearchParams } from "@/helpers/getSearchParams";
import { localStorageSavedLanguage } from "@/helpers/localStorageSavedLanguage";
import { renderApiErrorMessage } from "@/helpers/renderApiErrorMessage";

import { useStore } from "@/store/useStore";

const App: FC = () => {
  const searchParams = getSearchParams();
  const token = searchParams.get("token");
  const { t } = useTranslation();

  const { updateJwtToken, updateLanguage, updateOrg } = useStore();
  const [language, setLanguage] = useState<string>("");

  const {
    data: orgData,
    error: orgError,
    isFetching: isOrgFetching,
    isLoading: isOrgLoading,
  } = useSep24DepositInit(token);

  useEffect(() => {
    // Detect user’s language.
    // For example: "en", "en-US", "uk", "uk-UA". We need only the "en" or "uk"
    // part.
    const detectedLang = navigator.language.split("-")[0];

    // Get saved language from local storage if it exists.
    const savedLang = localStorageSavedLanguage.get();

    setLanguage(
      savedLang || SUPPORTED_LANGUAGES.includes(detectedLang)
        ? detectedLang
        : "en"
    );
  }, []);

  useEffect(() => {
    i18next.changeLanguage(language);
    updateLanguage(language);
  }, [language, updateLanguage]);

  useEffect(() => {
    updateJwtToken(token || "");
  }, [token, updateJwtToken]);

  useEffect(() => {
    if (orgData) {
      const {
        privacy_policy_link,
        organization_name,
        is_registered,
        truncated_contact_info,
        is_recaptcha_disabled,
        recaptcha_site_key,
        captcha_type,
      } = orgData;

      updateOrg({
        privacy_policy_link,
        organization_name,
        is_registered,
        truncated_contact_info,
        is_recaptcha_disabled: Boolean(is_recaptcha_disabled),
        recaptcha_site_key,
        captcha_type,
      });
    }
  }, [orgData, updateOrg]);

  const renderContent = () => {
    // Loading
    if (isOrgFetching || isOrgLoading) {
      return (
        <Box
          gap="sm"
          justify="center"
          align="center"
          addlClassName="FullScreenLoader"
        >
          <Loader size="2rem" />
        </Box>
      );
    }

    // Error
    if (orgError) {
      return (
        <div className="Wallet__content">
          <Notification variant="error" title={t("generic.error")}>
            {renderApiErrorMessage(t, orgError)}
          </Notification>
        </div>
      );
    }

    // Content
    return (
      <BrowserRouter basename={Routes.BASE}>
        <RouterRoutes>
          <Route path={Routes.ROOT} element={<SelectVerificationMethod />} />
          <Route path={Routes.START} element={<SelectVerificationMethod />} />
          <Route path={Routes.ENTER_PASSCODE} element={<PasscodeEntry />} />
          <Route path={Routes.SUCCESS} element={<Success />} />
          <Route
            path={Routes.ALREADY_REGISTERED}
            element={<AlreadyRegistered />}
          />
          {/* Add a catch-all route that redirects to the start page */}
          <Route path="*" element={<SelectVerificationMethod />} />
        </RouterRoutes>
      </BrowserRouter>
    );
  };

  return (
    <>
      {/* Header */}
      <Box
        gap="lg"
        direction="row"
        align="center"
        justify="space-between"
        addlClassName="Wallet__header"
      >
        <Box gap="sm" addlClassName="Wallet__language">
          <Select
            fieldSize="sm"
            id="language"
            onChange={(e) => {
              const lang = e.target.value;

              setLanguage(lang);
              localStorageSavedLanguage.set(lang);
            }}
          >
            <option value="en">English</option>
            <option value="ua">Українська</option>
            <option value="es">Español</option>
            <option value="ru">Русский</option>
            <option value="ht">Kreyòl Ayisyen</option>
          </Select>
        </Box>

        <ThemeSwitch storageKeyId={LOCAL_STORAGE_WALLET_THEME} />
      </Box>

      {/* Content */}
      {renderContent()}
    </>
  );
};

export default App;
