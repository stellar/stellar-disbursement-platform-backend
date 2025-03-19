import { FC, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Button,
  Heading,
  Input,
  Link,
  Text,
} from "@stellar/design-system";
import ReCaptcha from "react-google-recaptcha";

import { Box } from "@/components/Box";
import { ContentLayout } from "@/components/ContentLayout";
import { useStore } from "@/store/useStore";
import { RECAPTCHA_SITE_KEY } from "@/config/settings";

export const PasscodeEntry: FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user, jwtToken } = useStore();

  const [otp, setOtp] = useState("");
  const [verification, setVerification] = useState("");

  const reCaptchaRef = useRef<ReCaptcha>(null);
  const [reCaptchaToken, setReCaptchaToken] = useState<string | null>(null);

  // TODO: handle verification
  const handleVerification = () => {
    if (!(otp || verification || user.verification_field || reCaptchaToken)) {
      return;
    }

    let ver = verification;

    if (user.verification_field === "YEAR_MONTH") {
      const dateString = verification.split("-");
      ver = `${dateString[0]}-${dateString[1]}`;
    }

    const submitData = {
      phone_number: user.phone_number,
      email: user.email,
      otp,
      verification: ver,
      verification_field: user.verification_field,
      recaptcha_token: reCaptchaToken,
      token: jwtToken,
    };

    console.log(">>> submit data: ", submitData);

    // Handle verification success logic here
    alert("Verification successful!");
  };

  // TODO: handle resend otp
  const handleResendOtp = () => {
    alert("New OTP sent!");
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
            <Link onClick={handleResendOtp}> {t("generic.resendOtp")}</Link>
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
              // disabled={}
            >
              {t("generic.goBack")}
            </Button>

            <Button
              size="lg"
              variant="secondary"
              onClick={handleVerification}
              disabled={isSubmitDisabled()}
              // isLoading={}
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
            }}
          />
        </Box>
      </Box>
    </ContentLayout>
  );
};
