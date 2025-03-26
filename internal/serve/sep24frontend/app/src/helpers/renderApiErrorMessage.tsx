import { ApiError } from "@/types/types";
import { translatedApiErrorMessage } from "@/helpers/translatedApiErrorMessage";

/**
 * Renders a React component that displays the API error message to the user.
 * @param apiError - The API error object.
 * @returns The rendered API error message.
 */
export const renderApiErrorMessage = (apiError: ApiError) => {
  return (
    <>
      {translatedApiErrorMessage(apiError)}
      {apiError?.extras ? (
        <ul>
          {Object.entries(apiError.extras).map(([key, value]) => (
            <li key={`error-${key}`}>{value}</li>
          ))}
        </ul>
      ) : null}
    </>
  );
};
