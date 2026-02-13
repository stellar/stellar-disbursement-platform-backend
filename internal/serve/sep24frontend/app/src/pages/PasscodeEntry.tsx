import { FC, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Button,
  Heading,
  Input,
  Link,
  Notification,
  Text,
} from "@stellar/design-system";
import ReCaptcha from "react-google-recaptcha";

import { Box } from "@/components/Box";
import { ContentLayout } from "@/components/ContentLayout";

import { useStore } from "@/store/useStore";
import { useCaptcha } from "@/hooks/useCaptcha";
import { Routes } from "@/config/settings";
import { translatedApiErrorMessage } from "@/helpers/translatedApiErrorMessage";
import { getSearchParams } from "@/helpers/getSearchParams";
import { useIsEmptyUser } from "@/hooks/useIsEmptyUser";
import { useSep24DepositOtp } from "@/query/useSep24DepositOtp";
import { useSep24DepositVerification } from "@/query/useSep24DepositVerification";

export const PasscodeEntry: FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const searchParams = getSearchParams().toString();
  const { user, jwtToken } = useStore();
  useIsEmptyUser();

  const [otp, setOtp] = useState("");
  const [verification, setVerification] = useState("");

  const reCaptchaRef = useRef<ReCaptcha>(null);
  const captcha = useCaptcha(reCaptchaRef);

  type ViewMessage = {
    type: "error" | "success";
    title: string;
    message: string;
    timestamp: number;
  };

  const [viewMessage, setViewMessage] = useState<ViewMessage | null>();

  const {
    isSuccess: isOtpSuccess,
    error: otpError,
    isPending: isOtpPending,
    mutate: otpSubmit,
  } = useSep24DepositOtp();

  const {
    isSuccess: isVerifySuccess,
    error: verifyError,
    isPending: isVerifyPending,
    mutate: verifySubmit,
  } = useSep24DepositVerification();

  const clearMessages = () => {
    if (viewMessage) {
      setViewMessage(null);
    }
  };

  const scrollToTop = () => {
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  // OTP success
  useEffect(() => {
    if (isOtpSuccess) {
      setViewMessage({
        type: "success",
        title: t("enterPasscode.resendOtpSuccessTitle"),
        message: t("enterPasscode.resendOtpSuccessMessage"),
        timestamp: new Date().getTime(),
      });

      scrollToTop();
      captcha.resetCaptcha();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOtpSuccess, t]);

  // OTP error
  useEffect(() => {
    if (otpError) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: translatedApiErrorMessage(t, otpError),
        timestamp: new Date().getTime(),
      });

      scrollToTop();
      captcha.resetCaptcha();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [otpError, t]);

  // Verify success
  useEffect(() => {
    if (isVerifySuccess) {
      navigate({ pathname: Routes.SUCCESS, search: searchParams });
      captcha.resetCaptcha();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isVerifySuccess, navigate, searchParams]);

  // Verify error
  useEffect(() => {
    if (verifyError) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: translatedApiErrorMessage(t, verifyError),
        timestamp: new Date().getTime(),
      });

      captcha.resetCaptcha();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [verifyError, t]);

  const handleVerification = async () => {
    if (!(otp && verification && user.verification_field) || captcha.isPending) {
      return;
    }

    let recaptchaToken = "";
    try {
      recaptchaToken = await captcha.getToken("verify_registration");
    } catch (err) {
      console.error("reCAPTCHA failed:", err);
      return;
    }

    const formattedVerification =
      user.verification_field === "YEAR_MONTH"
        ? verification.substring(0, 7)
        : verification;

    verifySubmit({
      phone_number: user.phone_number,
      email: user.email,
      otp,
      verification: formattedVerification,
      verification_field: user.verification_field,
      recaptcha_token: recaptchaToken || undefined,
      token: jwtToken,
    });
  };

  const handleResendOtp = async () => {
    if (!jwtToken) {
      return;
    }

    if (captcha.isPending) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: t("generic.errorReCaptchaRequired"),
        timestamp: new Date().getTime(),
      });

      scrollToTop();

      return;
    }

    let recaptchaToken = "";
    try {
      recaptchaToken = await captcha.getToken("resend_otp");
    } catch (err) {
      console.error("reCAPTCHA failed:", err);
      return;
    }

    const submitData = {
      phone_number: user.phone_number,
      email: user.email,
      recaptcha_token: recaptchaToken || undefined,
    };

    otpSubmit({ token: jwtToken, ...submitData });
  };

  const renderVerificationInput = () => {
    const { verification_field } = user;

    let inputProps = {
      id: "",
      label: "",
      type: "",
    };

    switch (verification_field) {
      case "DATE_OF_BIRTH":
        inputProps = {
          id: "input-date-of-birth",
          label: t("enterPasscode.verDateOfBirth"),
          type: "date",
        };
        break;
      case "NATIONAL_ID_NUMBER":
        inputProps = {
          id: "input-national-id",
          label: t("enterPasscode.verNationalIdNumber"),
          type: "text",
        };
        break;
      case "PIN":
        inputProps = {
          id: "input-pin",
          label: t("enterPasscode.verPin"),
          type: "text",
        };
        break;
      case "YEAR_MONTH":
        inputProps = {
          id: "input-year-month",
          label: t("enterPasscode.verYearMonth"),
          type: "date",
        };
        break;
      default:
        return null;
    }

    return (
      <Input
        {...inputProps}
        fieldSize="lg"
        value={verification}
        onChange={(e) => {
          clearMessages();
          setVerification(e.target.value);
        }}
      />
    );
  };

  return (
    <ContentLayout
      footer={
        <>
          <Box
            gap="md"
            direction="row"
            justify="center"
            addlClassName="Wallet__passcodeEntry__resendOtp"
          >
            <Link
              onClick={(e) => {
                e.preventDefault();
                clearMessages();
                handleResendOtp();
              }}
              isDisabled={isOtpPending}
            >
              {t("generic.resendOtp")}
            </Link>
          </Box>

          <Box
            gap="md"
            direction="row"
            align="center"
            justify="space-between"
            addlClassName="Wallet__footer__inlineButtons"
          >
            <Button
              size="lg"
              variant="tertiary"
              onClick={() => {
                clearMessages();
                navigate(-1);
              }}
              disabled={isVerifyPending}
            >
              {t("generic.goBack")}
            </Button>

            <Button
              size="lg"
              variant="secondary"
              onClick={() => {
                clearMessages();
                handleVerification();
              }}
              disabled={!(otp && verification) || captcha.isPending}
              isLoading={isVerifyPending}
            >
              {t("generic.continue")}
            </Button>
          </Box>
        </>
      }
    >
      <Box gap="md">
        <Heading as="h1" size="sm">
          {t("enterPasscode.title")}
        </Heading>

        {viewMessage ? (
          <Notification
            title={viewMessage.title}
            variant={viewMessage.type}
            isFilled
          >
            {viewMessage.message}
          </Notification>
        ) : null}

        <Text as="div" size="md">
          {t("enterPasscode.message")}
        </Text>

        <Text as="div" size="md">
          {t("enterPasscode.messageNoOtp")}
        </Text>

        <Alert variant="warning" placement="inline">
          {t("enterPasscode.warning")}
        </Alert>

        <Box gap="lg" addlClassName="Wallet__passcodeEntry__inputs">
          <Input
            id="input-otp"
            type="number"
            label={t("enterPasscode.passcode")}
            value={otp}
            fieldSize="lg"
            onChange={(e) => {
              clearMessages();
              setOtp(e.target.value);
            }}
          />

          {renderVerificationInput()}

          {captcha.isV2 && captcha.siteKey && (
            <ReCaptcha
              ref={reCaptchaRef}
              size="normal"
              sitekey={captcha.siteKey}
              onChange={(token) => {
                clearMessages();
                captcha.onRecaptchaV2Change(token);
              }}
            />
          )}
        </Box>
      </Box>
    </ContentLayout>
  );
};
