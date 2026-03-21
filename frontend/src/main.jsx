import { ClerkProvider } from "@clerk/react";
import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./index.css";

const clerkPublishableKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;
const clerkProxyURL = (import.meta.env.VITE_CLERK_PROXY_URL || "").trim();

if (!clerkPublishableKey) {
  throw new Error("Add VITE_CLERK_PUBLISHABLE_KEY to the environment");
}

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <ClerkProvider publishableKey={clerkPublishableKey} proxyUrl={clerkProxyURL || undefined}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ClerkProvider>
  </React.StrictMode>,
);
