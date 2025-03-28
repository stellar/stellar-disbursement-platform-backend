import { FC, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Trans, useTranslation } from "react-i18next";
import { Button, Heading, Text } from "@stellar/design-system";

import { Box } from "@/components/Box";
import { ContentLayout } from "@/components/ContentLayout";
import { Routes } from "@/config/settings";
import { getSearchParams } from "@/helpers/getSearchParams";
import { useStore } from "@/store/useStore";

export const Success: FC = () => {
  const { t } = useTranslation();
  const { user } = useStore();
  const navigate = useNavigate();
  const searchParams = getSearchParams().toString();

  // Redirect to /start if user is not set
  useEffect(() => {
    if (Object.keys(user).length === 0) {
      navigate({ pathname: Routes.START, search: searchParams });
    }
  }, [user, navigate, searchParams]);

  return (
    <ContentLayout
      footer={
        <Button
          size="lg"
          variant="secondary"
          onClick={() => {
            window.close();

            // Purpose: to let other windows know that this window has been closed
            postMessage("close");
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
