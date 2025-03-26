import { Link } from "@stellar/design-system";
import { Trans } from "react-i18next";

import { Box } from "@/components/Box";
import { useStore } from "@/store/useStore";

export const ContentLayout = ({
  children,
  footer,
}: {
  children: React.ReactElement;
  footer?: React.ReactElement;
}) => {
  const { org } = useStore();

  return (
    <>
      {/* Content */}
      <div className="Wallet__content">
        <div className="Wallet__content__box">{children}</div>
      </div>

      {/* Footer */}
      <Box gap="lg" addlClassName="Wallet__footer">
        {footer ? <Box gap="sm">{footer}</Box> : null}

        <div>
          <Trans
            i18nKey="footer.privacyMessage"
            values={{ orgName: org.organization_name }}
            components={{
              Link: <Link href={org.privacy_policy_link}>Privacy Policy</Link>,
            }}
          />
        </div>
      </Box>
    </>
  );
};
