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
import { ru, en } from "intl-tel-input/i18n";

import ReCaptcha from "react-google-recaptcha";

import { ContentLayout } from "@/components/ContentLayout";
import { Box } from "@/components/Box";
import { ExpandBox } from "@/components/ExpandBox";

import { getSearchParams } from "@/helpers/getSearchParams";
import { renderApiErrorMessage } from "@/helpers/renderApiErrorMessage";
import { Routes } from "@/config/settings";
import { useSep24DepositOtp } from "@/query/useSep24DepositOtp";

import { useStore } from "@/store/useStore";
import { useCaptcha } from "@/hooks/useCaptcha";
import { VerificationMethod } from "@/types/types";

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
  const captcha = useCaptcha(reCaptchaRef);

  // Redirect to /already-registered if user is registered
  useEffect(() => {
    if (org.is_registered) {
      navigate({ pathname: Routes.ALREADY_REGISTERED, search: searchParams });
    }
  }, [org.is_registered, navigate, searchParams]);

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
      ua: ru,
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
            // If the IP check fails, set default to USA
            .catch(() => callback("US"));
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

  // Reset state when selected method changes (and not the language)
  useEffect(() => {
    setInputEmail("");
    setInputEmailError("");

    iti?.setNumber("");
    setInputPhoneError("");
    // Not including iti to keep entered value when language changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedMethod]);

  // OTP response
  useEffect(() => {
    if (otpData) {
      updateUser({
        phone_number: iti?.getNumber(),
        email: inputEmail,
        verification_field: otpData.verification_field,
      });
      navigate({ pathname: Routes.ENTER_PASSCODE, search: searchParams });
      captcha.resetCaptcha();
    }
    // Not including iti and inputEmail
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [otpData, navigate, searchParams, updateUser]);

  const handleSubmit = async () => {
    if (!jwtToken || captcha.isPending) {
      return;
    }

    let recaptchaToken = "";
    try {
      recaptchaToken = await captcha.getToken("submit_otp");
    } catch (err) {
      console.error("reCAPTCHA failed:", err);
      return;
    }

    const submitData = {
      phone_number: iti?.getNumber() || undefined,
      email: inputEmail || undefined,
      recaptcha_token: recaptchaToken || undefined,
    };

    otpSubmit({ token: jwtToken, ...submitData });
  };

  const validatePhoneNumber = () => {
    if (!iti?.getNumber()) {
      return;
    }

    const isValid = iti?.isValidNumber();
    setInputPhoneError(
      isValid ? false : t("selectVerification.phoneErrorMessage"),
    );
  };

  const validateEmail = () => {
    if (!inputEmail) {
      return;
    }

    const isValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(inputEmail);
    setInputEmailError(
      isValid ? false : t("selectVerification.emailErrorMessage"),
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

        {captcha.isV2 && captcha.siteKey && (
          <ReCaptcha
            ref={reCaptchaRef}
            size="normal"
            sitekey={captcha.siteKey}
            onChange={(token) => {
              captcha.onRecaptchaV2Change(token);
            }}
          />
        )}
      </Box>
    );
  };

  const isSubmitDisabled = () => {
    if (!selectedMethod) {
      return true;
    }

    if (selectedMethod === "email") {
      if (inputEmailError !== false || !inputEmail) {
        return true;
      }
    } else if (selectedMethod === "phone") {
      if (inputPhoneError !== false || !iti?.isValidNumber()) {
        return true;
      }
    }

    if (captcha.isPending) {
      return true;
    }

    if (!jwtToken) {
      return true;
    }

    return false;
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
