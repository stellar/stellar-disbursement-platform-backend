import { useCallback, useEffect, useRef, useState } from "react";
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
      render?: (container: string | HTMLElement, options: object) => number;
      reset?: (widgetId?: number) => void;
    };
  }
}

export const useCaptcha = () => {
  const { org } = useStore();

  const reCaptchaRef = useRef<ReCaptcha>(null);
  const [reCaptchaToken, setReCaptchaToken] = useState<string | null>(null);
  const [isV3ScriptLoaded, setIsV3ScriptLoaded] = useState(false);

  const isV3 = org.captcha_type === "GOOGLE_RECAPTCHA_V3";
  const isDisabled = org.is_recaptcha_disabled;
  const siteKey = org.recaptcha_site_key;

  useEffect(() => {
    if (!isV3 || !siteKey || isDisabled) {
      return;
    }

    const scriptId = "recaptcha-v3-script";
    if (document.getElementById(scriptId)) {
      setIsV3ScriptLoaded(true);
      return;
    }

    const script = document.createElement("script");
    script.id = scriptId;
    script.src = `https://www.google.com/recaptcha/api.js?render=${siteKey}`;
    script.async = true;
    script.onload = () => {
      window.grecaptcha.ready(() => {
        setIsV3ScriptLoaded(true);
      });
    };
    document.head.appendChild(script);

    return () => {
      const existingScript = document.getElementById(scriptId);
      if (existingScript) {
        existingScript.remove();
      }
    };
  }, [isV3, siteKey, isDisabled]);

  const isRecaptchaPending = useCallback(() => {
    if (isDisabled) {
      return false;
    }
    if (isV3) {
      return !isV3ScriptLoaded;
    }
    return !reCaptchaToken;
  }, [isDisabled, isV3, isV3ScriptLoaded, reCaptchaToken]);

  const executeV3 = useCallback(
    async (action: string): Promise<string | null> => {
      if (!isV3 || isDisabled || !siteKey) {
        return reCaptchaToken;
      }
      try {
        return await window.grecaptcha.execute(siteKey, { action });
      } catch (error) {
        console.error("reCAPTCHA v3 execution failed:", error);
        return null;
      }
    },
    [isV3, isDisabled, siteKey, reCaptchaToken],
  );

  const resetCaptcha = useCallback(() => {
    reCaptchaRef.current?.reset();
  }, []);

  return {
    reCaptchaRef,
    reCaptchaToken,
    setReCaptchaToken,
    isV3,
    isDisabled,
    siteKey,
    isRecaptchaPending,
    executeV3,
    resetCaptcha,
  };
};
