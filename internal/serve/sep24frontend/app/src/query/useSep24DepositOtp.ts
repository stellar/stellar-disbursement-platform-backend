import { useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { SDP_API_ENDPOINT } from "@/config/settings";
import { throwUnexpectedError } from "@/helpers/throwUnexpectedError";
import { ApiError, Sep24DepositOtpResponse } from "@/types/types";

/**
 * Submit user’s verification method to get OTP.
 */
type Sep24DepositOtpProps = {
  token: string;
  phone_number: string | undefined;
  email: string | undefined;
  recaptcha_token: string | undefined;
};

export const useSep24DepositOtp = () => {
  const { t } = useTranslation();

  const mutation = useMutation<
    Sep24DepositOtpResponse,
    ApiError,
    Sep24DepositOtpProps
  >({
    mutationFn: async ({
      token,
      phone_number,
      email,
      recaptcha_token,
    }: Sep24DepositOtpProps) => {
      if (!token) {
        throw Error("Token is required.");
      }

      const data = {
        recaptcha_token,
        ...(phone_number ? { phone_number } : {}),
        ...(email ? { email } : {}),
      };

      try {
        const response = await fetch(
          `${SDP_API_ENDPOINT}/sep24-interactive-deposit/otp`,
          {
            method: "POST",
            headers: {
              Authorization: `Bearer ${token}`,
              "Content-Type": "application/json",
            },
            body: JSON.stringify(data),
          }
        );

        // Check if status is an error:
        if (!response.ok) {
          const errorData = await response.json().catch(() => null);
          throw errorData;
        }

        return response.json();
      } catch (e: any) {
        if (e.error) {
          throw e;
        }

        throwUnexpectedError(t, e);
      }
    },
  });

  return mutation;
};
