import { FC, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Alert,
  Button,
  Heading,
  RadioButton,
  Text,
} from "@stellar/design-system";

import { ContentLayout } from "@/components/ContentLayout";
import { Box } from "@/components/Box";

import { Routes } from "@/config/settings";
import { VerificationMethod } from "@/types/types";

// TODO: handle verification method fields (show dynamically)
// TODO: add ReCaptcha

export const SelectVerificationMethod: FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [selectedMethod, setSelectedMethod] =
    useState<VerificationMethod>(null);

  const handleContinue = () => {
    if (selectedMethod === "email") {
      navigate(Routes.VERIFY_EMAIL);
    } else if (selectedMethod === "phone") {
      navigate(Routes.VERIFY_PHONE);
    }
  };

  return (
    <ContentLayout
      footer={
        <Button
          size="md"
          variant="secondary"
          onClick={handleContinue}
          disabled={!selectedMethod}
        >
          {t("generic.continue")}
        </Button>
      }
    >
      <Box gap="md">
        <Heading as="h1" size="sm">
          {t("selectVerification.title")}
        </Heading>
        <Text as="div" size="md">
          {t("selectVerification.message")}
        </Text>

        <Box gap="sm">
          <RadioButton
            id="verification-method-phone"
            name="verification-method"
            label={t("generic.phoneNumber")}
            fieldSize="lg"
            checked={selectedMethod === "phone"}
            onChange={() => setSelectedMethod("phone")}
          />
          <RadioButton
            id="verification-method-email"
            name="verification-method"
            label={t("generic.email")}
            fieldSize="lg"
            checked={selectedMethod === "email"}
            onChange={() => setSelectedMethod("email")}
          />
        </Box>

        <Alert variant="warning" placement="inline">
          {t("selectVerification.warning")}
        </Alert>
      </Box>
    </ContentLayout>
  );
};
