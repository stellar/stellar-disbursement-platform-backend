import { useQuery } from "@tanstack/react-query";
import { Sep24DepositInitResponse } from "@/types/types";

/**
 * Initial SEP-24 deposit call made when the app launches to get org data and
 * user’s registration status.
 */
export const useSep24DepositInit = (token: string | null) => {
  const query = useQuery<Sep24DepositInitResponse>({
    queryKey: ["useSep24DepositInit", token],
    queryFn: async () => {
      if (!token) {
        throw Error("Token is required.");
      }

      try {
        const response = await fetch(`/sep24-interactive-deposit/init`, {
          method: "GET",
          headers: {
            Authorization: `Bearer ${token}`,
          },
        });

        const responseJson = await response.json();

        if (responseJson.error) {
          throw responseJson.error || "Unknown error.";
        }

        return responseJson;
      } catch (e) {
        throw `There was an error initializing: ${e}`;
      }
    },
    enabled: Boolean(token),
  });

  return query;
};
