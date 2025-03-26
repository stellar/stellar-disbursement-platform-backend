import { ApiError } from "@/types/types";
import { TFunction } from "i18next";

/**
 * Gets the translated text to be displayed to the user based on the error code. If no error code is provided, then the
 * original English error message is returned.
 * @returns The error message.
 */
export const translatedApiErrorMessage = (
  t: TFunction,
  { error, error_code }: ApiError
): string => {
  if (!error_code) {
    return error;
  }

  const translatedText = t("errorCodes." + error_code);
  if (!translatedText) {
    return error;
  }

  return translatedText;
};
