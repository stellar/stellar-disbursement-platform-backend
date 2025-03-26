import { useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { SDP_API_ENDPOINT } from "@/config/settings";
import { throwUnexpectedError } from "@/helpers/throwUnexpectedError";
import { ApiError } from "@/types/types";

/**
 * Submit verify receiver.
 */
type Sep24DepositVerificationProps = {
  token: string;
  phone_number: string | undefined;
  email: string | undefined;
  recaptcha_token: string | undefined;
  otp: string;
  verification: string;
  verification_field: string;
};

export const useSep24DepositVerification = () => {
  const { t } = useTranslation();

  const mutation = useMutation<
    { message: string },
    ApiError,
    Sep24DepositVerificationProps
  >({
    mutationFn: async ({
      token,
      phone_number,
      email,
      recaptcha_token,
      otp,
      verification,
      verification_field,
    }: Sep24DepositVerificationProps) => {
      if (!token) {
        throw Error("Token is required.");
      }

      const data = {
        recaptcha_token,
        ...(phone_number ? { phone_number } : {}),
        ...(email ? { email } : {}),
        otp,
        verification,
        verification_field,
      };

      try {
        const response = await fetch(
          `${SDP_API_ENDPOINT}/sep24-interactive-deposit/verification`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${token}`,
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
