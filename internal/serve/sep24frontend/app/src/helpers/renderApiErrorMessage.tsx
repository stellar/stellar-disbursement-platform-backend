import { ApiError } from "@/types/types";

export const renderApiErrorMessage = (apiError: ApiError) => {
  return (
    <>
      {apiError.error}
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
