import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { Toaster } from "./components/Toaster";
import "./index.css";

const queries = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <QueryClientProvider client={queries}>
        <Toaster>
          <App />
        </Toaster>
      </QueryClientProvider>
    </StrictMode>,
  );
}
