import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppRouter } from "./router";
import { AppDialogProvider } from "./confirm";
import { initTheme } from "./theme";
import "sweetalert2/dist/sweetalert2.min.css";
import "./index.css";

initTheme();

const qc = new QueryClient();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={qc}>
      <AppDialogProvider>
        <AppRouter />
      </AppDialogProvider>
    </QueryClientProvider>
  </StrictMode>
);
