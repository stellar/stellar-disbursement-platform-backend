import { useTranslation } from "react-i18next";

const { t } = useTranslation();

export const throwUnexpectedError = (e: any) => {
  console.log("unexpected error: ", e);
  throw {
    error: t("generic.errorMessage"),
    error_code: "500_0",
  };
};
