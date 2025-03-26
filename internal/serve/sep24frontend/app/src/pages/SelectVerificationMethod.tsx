import { FC, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Button,
  Heading,
  Input,
  Notification,
  RadioButton,
  Text,
} from "@stellar/design-system";

import { useTranslation } from "react-i18next";
import intlTelInput, { Iti } from "intl-tel-input";
import { uk, en } from "intl-tel-input/i18n";

import ReCaptcha from "react-google-recaptcha";

import { ContentLayout } from "@/components/ContentLayout";
import { Box } from "@/components/Box";
import { ExpandBox } from "@/components/ExpandBox";

import { getSearchParams } from "@/helpers/getSearchParams";
import { renderApiErrorMessage } from "@/helpers/renderApiErrorMessage";
import { Routes } from "@/config/settings";
import { useSep24DepositOtp } from "@/query/useSep24DepositOtp";

import { useStore } from "@/store/useStore";
import { VerificationMethod } from "@/types/types";

// TODO: persist entered values

export const SelectVerificationMethod: FC = () => {
  const { jwtToken, language, updateUser, org } = useStore();
  const { t } = useTranslation();

  const navigate = useNavigate();
  const searchParams = getSearchParams().toString();

  const [selectedMethod, setSelectedMethod] =
    useState<VerificationMethod>(null);
  const [inputEmail, setInputEmail] = useState("");

  const [inputPhoneError, setInputPhoneError] = useState<string | false>("");
  const [inputEmailError, setInputEmailError] = useState<string | false>("");

  const [iti, setIti] = useState<Iti | null>(null);
  const phoneInputRef = useRef<HTMLInputElement | null>(null);

  const reCaptchaRef = useRef<ReCaptcha>(null);
  const [reCaptchaToken, setReCaptchaToken] = useState<string | null>(null);

  // Redirect to already registered page if user is registered
  useEffect(() => {
    if (org.is_registered) {
      navigate({ pathname: Routes.ALREADY_REGISTERED });
    }
  }, [org.is_registered, navigate]);

  const {
    data: otpData,
    error: otpError,
    isPending: isOtpPending,
    mutate: otpSubmit,
    reset: resetOtp,
  } = useSep24DepositOtp();

  const clearErrors = () => {
    if (otpError) {
      resetOtp();
    }

    if (inputPhoneError) {
      setInputPhoneError("");
    }

    if (inputEmailError) {
      setInputEmailError("");
    }
  };

  // Initialize intlTelInput
  useEffect(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const intlTelInputDropdownLang: any = {
      en,
      uk,
    };

    if (phoneInputRef?.current) {
      const itiInit = intlTelInput(phoneInputRef.current, {
        loadUtils: () => import("intl-tel-input/utils"),
        separateDialCode: true,
        // Excluding Cuba, Iran, North Korea, and Syria
        excludeCountries: ["cu", "ir", "kp", "sy"],
        // Setting default country based on user's IP address
        initialCountry: "auto",
        // Get the country code from user's location
        geoIpLookup: (callback) => {
          fetch("https://ipapi.co/json")
            .then((res) => res.json())
            .then((data) => callback(data.country_code))
            .catch(() => callback(""));
        },
        i18n: intlTelInputDropdownLang[language] || en,
        fixDropdownWidth: false,
        containerClass: "Wallet__phoneInput",
        useFullscreenPopup: true,
      });

      setIti(itiInit);
    }

    return () => {
      iti?.destroy();
    };
    // Trigger change when global app language changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language]);

  // Reset state when selected method changes
  useEffect(() => {
    setInputEmail("");
    setInputEmailError("");

    iti?.setNumber("");
    setInputPhoneError("");
  }, [iti, selectedMethod]);

  // OTP response
  useEffect(() => {
    if (otpData) {
      updateUser({
        phone_number: iti?.getNumber(),
        email: inputEmail,
        verification_field: otpData.verification_field,
      });
      navigate({ pathname: Routes.ENTER_PASSCODE, search: searchParams });
      reCaptchaRef?.current?.reset();
    }
    // Not including iti and inputEmail
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [otpData, navigate, searchParams, updateUser]);

  const handleSubmit = () => {
    if (!jwtToken || !reCaptchaToken) {
      return;
    }

    const submitData = {
      phone_number: iti?.getNumber() || undefined,
      email: inputEmail || undefined,
      recaptcha_token: reCaptchaToken,
    };

    otpSubmit({ token: jwtToken, ...submitData });
  };

  const validatePhoneNumber = () => {
    if (!iti?.getNumber()) {
      return;
    }

    const isValid = iti?.isValidNumber();
    setInputPhoneError(
      isValid ? false : t("selectVerification.phoneErrorMessage")
    );
  };

  const validateEmail = () => {
    if (!inputEmail) {
      return;
    }

    const isValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(inputEmail);
    setInputEmailError(
      isValid ? false : t("selectVerification.emailErrorMessage")
    );
  };

  const renderInputs = () => {
    return (
      <Box gap="md">
        <div
          className="Wallet__verificationMethodBox"
          data-visible={selectedMethod === "email"}
        >
          <Input
            id="input-email"
            type="email"
            value={inputEmail}
            fieldSize="lg"
            placeholder="email@email.com"
            onChange={(e) => {
              clearErrors();
              setInputEmail(e.target.value.trim());
            }}
            onBlur={() => {
              validateEmail();
            }}
            error={inputEmailError}
          />
        </div>

        <div
          className="Wallet__verificationMethodBox Wallet__phoneInput"
          data-visible={selectedMethod === "phone"}
        >
          <input
            ref={phoneInputRef}
            id="input-phone-number"
            type="tel"
            placeholder="(506) 234-5678"
            onBlur={() => {
              validatePhoneNumber();
            }}
            onChange={clearErrors}
            data-error={Boolean(inputPhoneError)}
          />
          {inputPhoneError ? (
            <div className="FieldNote FieldNote--error FieldNote--lg">
              {inputPhoneError}
            </div>
          ) : null}
        </div>

        {/* TODO: match recaptcha theme */}
        {org.recaptcha_site_key && (
          <ReCaptcha
            ref={reCaptchaRef}
            size="normal"
            sitekey={org.recaptcha_site_key}
            onChange={(token) => {
              setReCaptchaToken(token);
            }}
          />
        )}
      </Box>
    );
  };

  const isSubmitDisabled = () => {
    let isDisabled = false;

    if (!selectedMethod) {
      isDisabled = true;
    }

    if (selectedMethod === "email") {
      isDisabled = inputEmailError !== false || !inputEmail;
    } else if (selectedMethod === "phone") {
      isDisabled = inputPhoneError !== false || !iti?.isValidNumber();
    }

    if (!reCaptchaToken) {
      isDisabled = true;
    }

    if (!jwtToken) {
      isDisabled = true;
    }

    return isDisabled;
  };

  return (
    <ContentLayout
      footer={
        <Button
          size="lg"
          variant="secondary"
          onClick={handleSubmit}
          disabled={isSubmitDisabled()}
          isLoading={isOtpPending}
        >
          {t("generic.continue")}
        </Button>
      }
    >
      <Box gap="md">
        {!jwtToken ? (
          <Notification variant="error" title="Attention" isFilled>
            {t("selectVerification.missingAuthToken")}
          </Notification>
        ) : null}

        <Heading as="h1" size="sm">
          {t("selectVerification.title")}
        </Heading>

        <Text as="div" size="md">
          {t("selectVerification.message")}
        </Text>

        <Alert variant="warning" placement="inline">
          {t("selectVerification.warning")}
        </Alert>

        <Box gap="sm">
          <RadioButton
            id="verification-method-phone"
            name="verification-method"
            label={t("generic.phoneNumber")}
            fieldSize="lg"
            checked={selectedMethod === "phone"}
            onChange={() => {
              clearErrors();
              setSelectedMethod("phone");
            }}
          />
          <RadioButton
            id="verification-method-email"
            name="verification-method"
            label={t("generic.email")}
            fieldSize="lg"
            checked={selectedMethod === "email"}
            onChange={() => {
              clearErrors();
              setSelectedMethod("email");
            }}
          />
        </Box>

        <ExpandBox offsetTop="sm" isExpanded={Boolean(selectedMethod)}>
          {renderInputs()}
        </ExpandBox>

        {otpError ? (
          <Alert variant="error" placement="inline" title={t("generic.error")}>
            {renderApiErrorMessage(t, otpError)}
          </Alert>
        ) : null}
      </Box>
    </ContentLayout>
  );
};
