import { TFunction } from "i18next";

/**
 * Throws an unexpected error.
 * @param t - The translation function.
 * @param e - The error object.
 */
export const throwUnexpectedError = (t: TFunction, e: any) => {
  console.log("unexpected error: ", e);
  throw {
    error: t("generic.errorMessage"),
    error_code: "500_0",
  };
};
