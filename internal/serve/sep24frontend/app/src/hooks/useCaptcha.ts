import React, { useCallback, useEffect, useState } from "react";
import ReCaptcha from "react-google-recaptcha";
import { useStore } from "@/store/useStore";

declare global {
  interface Window {
    grecaptcha: {
      ready: (callback: () => void) => void;
      execute: (
        siteKey: string,
        options: { action: string },
      ) => Promise<string>;
    };
  }
}

let scriptLoadingPromise: Promise<void> | null = null;

const isV3ScriptLoaded = (): boolean => {
  return !!document.querySelector(`script[src*="recaptcha/api.js?render="]`);
};

const loadRecaptchaV3Script = (siteKey: string): Promise<void> => {
  if (isV3ScriptLoaded()) {
    return Promise.resolve();
  }

  if (scriptLoadingPromise) {
    return scriptLoadingPromise;
  }

  scriptLoadingPromise = new Promise<void>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `https://www.google.com/recaptcha/api.js?render=${siteKey}`;
    script.async = true;
    script.onload = () => {
      window.grecaptcha.ready(() => resolve());
    };
    script.onerror = () => {
      scriptLoadingPromise = null;
      reject(new Error("Failed to load reCAPTCHA v3 script"));
    };
    document.head.appendChild(script);
  });

  return scriptLoadingPromise;
};

const executeRecaptchaV3 = async (
  siteKey: string,
  action: string,
): Promise<string> => {
  await loadRecaptchaV3Script(siteKey);
  return new Promise((resolve, reject) => {
    window.grecaptcha.ready(() => {
      window.grecaptcha.execute(siteKey, { action }).then(resolve).catch(reject);
    });
  });
};

export const useCaptcha = (
  recaptchaRef: React.RefObject<ReCaptcha | null>,
) => {
  const { org } = useStore();

  const [recaptchaToken, setRecaptchaToken] = useState("");

  const isCaptchaDisabled = org.is_recaptcha_disabled;
  const siteKey = org.recaptcha_site_key ?? "";
  const isV3 =
    !isCaptchaDisabled && org.captcha_type === "GOOGLE_RECAPTCHA_V3";
  const isV2 = !isCaptchaDisabled && !isV3;

  // Load v3 script when needed
  useEffect(() => {
    if (isV3 && siteKey) {
      loadRecaptchaV3Script(siteKey).catch((err) => {
        console.error("Failed to load reCAPTCHA v3:", err);
      });
    }
  }, [isV3, siteKey]);

  const onRecaptchaV2Change = (token: string | null) => {
    if (token) {
      setRecaptchaToken(token);
    }
  };

  /** Get the recaptcha token for form submission. For v3, executes at call time. */
  const getToken = useCallback(
    async (action: string): Promise<string> => {
      if (isCaptchaDisabled) {
        return "";
      }

      if (isV3 && siteKey) {
        return executeRecaptchaV3(siteKey, action);
      }

      return recaptchaToken;
    },
    [isCaptchaDisabled, isV3, siteKey, recaptchaToken],
  );

  const resetCaptcha = useCallback(() => {
    if (isV2) {
      recaptchaRef.current?.reset();
    }
    setRecaptchaToken("");
  }, [isV2, recaptchaRef]);

  const isPending = isV2 && !recaptchaToken;

  return {
    isV2,
    isV3,
    isCaptchaDisabled,
    isPending,
    recaptchaToken,
    siteKey,
    onRecaptchaV2Change,
    getToken,
    resetCaptcha,
  };
};
