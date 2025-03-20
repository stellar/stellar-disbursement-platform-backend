import { FC } from "react";
import { Trans, useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { Button, Heading, Text } from "@stellar/design-system";

import { Box } from "@/components/Box";
import { ContentLayout } from "@/components/ContentLayout";
import { Routes } from "@/config/settings";
import { useStore } from "@/store/useStore";

export const Success: FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user } = useStore();

  return (
    <ContentLayout
      footer={
        <Button
          size="lg"
          variant="secondary"
          onClick={() => {
            navigate(Routes.START);
          }}
        >
          {t("generic.goToHome")}
        </Button>
      }
    >
      <Box gap="md">
        <Heading as="h1" size="sm">
          {t("success.title")}
        </Heading>

        <Text as="div" size="md">
          <Trans
            i18nKey="success.message"
            values={{
              contactInfo: user.email || user.phone_number,
            }}
          />
        </Text>

        <Text as="div" size="md">
          {t("success.goToHomeMessage")}
        </Text>
      </Box>
    </ContentLayout>
  );
};
