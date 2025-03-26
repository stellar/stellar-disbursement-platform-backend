import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "@/App.tsx";
import { QueryProvider } from "@/query/QueryProvider.tsx";
import { StoreProvider } from "@/store/StoreProvider";

// Styles
import "@stellar/design-system/build/styles.min.css";
import "intl-tel-input/build/css/intlTelInput.min.css";
import "./index.scss";

// Translations
import "@/locales/i18n";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <StoreProvider>
      <QueryProvider>
        <App />
      </QueryProvider>
    </StoreProvider>
  </StrictMode>
);
