import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  envPrefix: ["VITE_", "WEB_PORT"],
  test: {
    environment: "jsdom",
    globals: true,
    clearMocks: true,
  },
});
