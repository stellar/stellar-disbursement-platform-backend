import { ApiError } from "@/types/types";
import { useTranslation } from "react-i18next";

const { t } = useTranslation();

/**
 * Gets the translated text to be displayed to the user based on the error code. If no error code is provided, then the
 * original English error message is returned.
 * @returns The error message.
 */
export const translatedApiErrorMessage = ({
  error,
  error_code,
}: ApiError): string => {
  if (!error_code) {
    return error;
  }

  const translatedText = t("errorCodes." + error_code);
  if (!translatedText) {
    return error;
  }

  return translatedText;
};
