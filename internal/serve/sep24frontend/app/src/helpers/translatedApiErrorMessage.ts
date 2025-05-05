import { ApiError } from "@/types/types";
import { TFunction } from "i18next";

/**
 * Gets the translated text to be displayed to the user based on the error code. If no error code is provided, then the
 * original English error message is returned. Includes extra details if provided in the error response.
 * @returns The error message.
 */
export const translatedApiErrorMessage = (
    t: TFunction,
    { error = 'Unknown error', error_code, extras }: ApiError
): string => {
  let baseMessage = error;

  if (error_code) {
    const translationKey = "errorCodes." + error_code;
    const translatedText = t(translationKey);
    if (translatedText !== translationKey) {
      baseMessage = translatedText;
    }
  }
  // Append extras if available
  if (extras && typeof extras === 'object' && Object.keys(extras).length > 0) {
    const extrasString = Object.entries(extras)
        .map(([key, value]) => `${key}: ${String(value)}`)
        .join(', ');
    baseMessage += ` (${extrasString})`;
  }

  return baseMessage;
};