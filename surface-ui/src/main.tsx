import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SurfaceThemeProvider } from "@dakasa-yggdrasil/surface-toolkit";
import "@dakasa-yggdrasil/surface-toolkit/styles";
import { App } from "./App";

// Conservative defaults that work well for an operator console:
// - staleTime 30s avoids hammering the API during navigation
// - retry: 1 surfaces real failures quickly without giving up immediately
// - refetchOnWindowFocus disabled because the console shouldn't refetch every
//   time the operator alt-tabs back from the native EFI console itself.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false }
  }
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <SurfaceThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter basename="/s/efi">
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </SurfaceThemeProvider>
  </StrictMode>
);
