import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { SDP_API_ENDPOINT } from "@/config/settings";
import { throwUnexpectedError } from "@/helpers/throwUnexpectedError";
import { ApiError, Sep24DepositInitResponse } from "@/types/types";

/**
 * Initial SEP-24 deposit call made when the app launches to get org data and
 * user’s registration status.
 */
export const useSep24DepositInit = (token: string | null) => {
  const { t } = useTranslation();

  const query = useQuery<Sep24DepositInitResponse, ApiError>({
    queryKey: ["useSep24DepositInit", token],
    queryFn: async () => {
      if (!token) {
        throw Error("Token is required.");
      }

      try {
        const response = await fetch(
          `${SDP_API_ENDPOINT}/sep24-interactive-deposit/info`,
          {
            method: "GET",
            headers: {
              Authorization: `Bearer ${token}`,
            },
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
    enabled: Boolean(token),
  });

  return query;
};
