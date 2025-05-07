import { ApiError } from "@/types/types";
import { TFunction } from "i18next";

/**
 * Gets the translated text to be displayed to the user based on the error code.
 * If no error code is provided, then the original English error message is returned.
 * Includes extra details if provided in the error response, prioritizing translation of extras_codes.
 * @returns The error message.
 */
export const translatedApiErrorMessage = (
    t: TFunction,
    { error = 'Unknown error', error_code, extras, extras_codes }: ApiError
): string => {
  let baseMessage = error;

  if (error_code) {
    const translationKey = "errorCodes." + error_code;
    const translatedText = t(translationKey);
    if (translatedText !== translationKey) {
      baseMessage = translatedText;
    }
  }

const details = extras_codes && extras_codes.length > 0
    ? extras_codes
        .map((code) => {
            const translationKey = "errorCodes." + code;
            const translatedExtra = t(translationKey);
            return translatedExtra !== translationKey ? translatedExtra : code;
        })
        .join(', ')
    : extras && Object.keys(extras).length > 0
        ? Object.entries(extras)
            .map(([key, value]) => `${key}: ${value}`)
            .join(', ')
        : '';


  if (details) {
    baseMessage += ` (${details})`;
  }

  return baseMessage;
};
