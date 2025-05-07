import { FC } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Button, Heading, Text } from "@stellar/design-system";

import { Box } from "@/components/Box";
import { ContentLayout } from "@/components/ContentLayout";
import { useStore } from "@/store/useStore";

export const AlreadyRegistered: FC = () => {
  const { t } = useTranslation();
  const {
    org: { truncated_contact_info },
  } = useStore();

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
          {t("alreadyRegistered.title")}
        </Heading>

        <Text as="div" size="md">
          <Trans
            i18nKey="alreadyRegistered.message"
            values={{
              contactInfo: truncated_contact_info,
            }}
          />
        </Text>

        <Text as="div" size="md">
          {t("alreadyRegistered.goToHomeMessage")}
        </Text>
      </Box>
    </ContentLayout>
  );
};
