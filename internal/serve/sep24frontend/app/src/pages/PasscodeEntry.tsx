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
import { RECAPTCHA_SITE_KEY, Routes } from "@/config/settings";
import { useSep24DepositOtp } from "@/query/useSep24DepositOtp";
import { useSep24DepositVerification } from "@/query/useSep24DepositVerification";

// TODO: clear view messages

export const PasscodeEntry: FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user, jwtToken } = useStore();

  const [otp, setOtp] = useState("");
  const [verification, setVerification] = useState("");

  const reCaptchaRef = useRef<ReCaptcha>(null);
  const [reCaptchaToken, setReCaptchaToken] = useState<string | null>(null);

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

  // OTP success
  useEffect(() => {
    if (isOtpSuccess) {
      setViewMessage({
        type: "success",
        title: t("enterPasscode.resendOtpSuccessTitle"),
        message: t("enterPasscode.resendOtpSuccessMessage"),
        timestamp: new Date().getTime(),
      });
    }
  }, [isOtpSuccess, t]);

  // OTP error
  useEffect(() => {
    if (otpError) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: otpError.error,
        timestamp: new Date().getTime(),
      });
    }
  }, [otpError, t]);

  // Verify success
  useEffect(() => {
    if (isVerifySuccess) {
      navigate(Routes.SUCCESS);
    }
  }, [isVerifySuccess, navigate]);

  // Verify error
  useEffect(() => {
    if (verifyError) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: verifyError.error,
        timestamp: new Date().getTime(),
      });
    }
  }, [verifyError, t]);

  const handleVerification = () => {
    if (!(otp && verification && user.verification_field && reCaptchaToken)) {
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
      recaptcha_token: reCaptchaToken,
      token: jwtToken,
    });
  };

  const handleResendOtp = () => {
    if (!jwtToken) {
      return;
    }

    if (!reCaptchaToken) {
      setViewMessage({
        type: "error",
        title: t("generic.error"),
        message: t("generic.errorReCaptchaRequired"),
        timestamp: new Date().getTime(),
      });

      return;
    }

    const submitData = {
      phone_number: user.phone_number,
      email: user.email,
      recaptcha_token: reCaptchaToken,
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
          setVerification(e.target.value);
        }}
      />
    );
  };

  const isSubmitDisabled = () => {
    if (!(otp && verification)) {
      return true;
    }

    if (!reCaptchaToken) {
      return true;
    }

    return false;
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
                navigate(-1);
              }}
              disabled={isVerifyPending}
            >
              {t("generic.goBack")}
            </Button>

            <Button
              size="lg"
              variant="secondary"
              onClick={handleVerification}
              disabled={isSubmitDisabled()}
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
            type="text"
            label={t("enterPasscode.passcode")}
            value={otp}
            fieldSize="lg"
            onChange={(e) => {
              setOtp(e.target.value);
            }}
          />

          {renderVerificationInput()}

          <ReCaptcha
            ref={reCaptchaRef}
            size="normal"
            sitekey={RECAPTCHA_SITE_KEY}
            onChange={(token) => {
              setReCaptchaToken(token);

              if (viewMessage) {
                setViewMessage(null);
              }
            }}
          />
        </Box>
      </Box>
    </ContentLayout>
  );
};
