FROM node:20-alpine

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# Vite reads VITE_* at build time. Inject frontend config during image build.
ARG VITE_API_BASE_URL=/api
ARG VITE_CLERK_PUBLISHABLE_KEY
RUN \
  VITE_API_BASE_URL="${VITE_API_BASE_URL}" \
  VITE_CLERK_PUBLISHABLE_KEY="${VITE_CLERK_PUBLISHABLE_KEY}" \
  npm run build

EXPOSE 5173

CMD ["npm", "run", "preview", "--", "--host", "0.0.0.0", "--port", "5173"]
