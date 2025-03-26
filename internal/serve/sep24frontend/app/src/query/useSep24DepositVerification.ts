import { useMutation } from "@tanstack/react-query";
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
  recaptcha_token: string;
  otp: string;
  verification: string;
  verification_field: string;
};

export const useSep24DepositVerification = () => {
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

        const responseJson = await response.json();

        if (responseJson.error) {
          throw responseJson;
        }

        return responseJson;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
      } catch (e: any) {
        if (e.error) {
          throw e;
        }

        throwUnexpectedError(e);
      }
    },
  });

  return mutation;
};
