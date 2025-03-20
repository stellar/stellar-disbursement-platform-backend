declare module "intl-tel-input/utils";

declare module "react-google-recaptcha" {
  import * as React from "react";

  interface ReCAPTCHAProps {
    sitekey: string;
    onChange?: (token: string | null) => void;
    grecaptcha?: object;
    theme?: "light" | "dark";
    type?: "image" | "audio";
    tabindex?: number;
    onExpired?: () => void;
    asyncScriptOnLoad?: () => void;
    onErrored?: () => void;
    stoken?: string;
    hl?: string;
    size?: "compact" | "normal" | "invisible";
    badge?: "bottomright" | "bottomleft" | "inline";
    isolated?: boolean;
  }

  class ReCAPTCHA extends React.Component<ReCAPTCHAProps> {
    reset(): void;
    execute(): void;
    executeAsync(): Promise<string | null>;
    getValue(): string | null;
    getWidgetId(): number | null;
  }

  export default ReCAPTCHA;
}
