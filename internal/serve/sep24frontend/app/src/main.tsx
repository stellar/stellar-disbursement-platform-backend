import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "@/App.tsx";
import { QueryProvider } from "@/query/QueryProvider.tsx";

// Styles
import "@stellar/design-system/build/styles.min.css";
import "./index.scss";

// Translations
import "@/locales/i18n";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryProvider>
      <App />
    </QueryProvider>
  </StrictMode>
);
