import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useStore } from "@/store/useStore";
import { Routes } from "@/config/settings";
import { getSearchParams } from "@/helpers/getSearchParams";

interface UseIsEmptyUserOptions {
  redirect?: boolean;
  redirectPath?: string;
}

export const useIsEmptyUser = ({
  redirect = true,
  redirectPath = Routes.START,
}: UseIsEmptyUserOptions = {}) => {
  const { user } = useStore();
  const navigate = useNavigate();
  const searchParams = getSearchParams().toString();

  const isUserEmpty = Object.keys(user).length === 0;

  useEffect(() => {
    if (redirect && isUserEmpty) {
      navigate({ pathname: redirectPath, search: searchParams });
    }
  }, [isUserEmpty, navigate, redirect, redirectPath, searchParams]);

  return isUserEmpty;
};
