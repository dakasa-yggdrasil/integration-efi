# syntax=docker/dockerfile:1.7

# Multi-stage Vite → nginx build for the federated surface-efi SPA.
#
# The build stage needs read access to GHCR npm to resolve
# @dakasa-yggdrasil/surface-toolkit. Pass NODE_AUTH_TOKEN as a build
# secret in CI (secrets.GITHUB_TOKEN auto-provides packages:read scope
# for the same-org private package).
FROM node:20-alpine AS build
ARG BUILD_BASE_PATH=/s/efi/
WORKDIR /app

# Copy dependency manifests first so this layer caches when only source
# code changes. .npmrc is required for the GHCR scope mapping.
COPY package*.json .npmrc ./

# npm install (not ci) because package-lock.json is not committed yet —
# local dev cannot reach GHCR npm without a PAT, so the lockfile is
# generated in CI. Once a lockfile is captured, this can flip to npm ci.
# NODE_AUTH_TOKEN is provided via BuildKit secret mount so it never
# lands in image layers.
RUN --mount=type=secret,id=npm_token,target=/run/secrets/npm_token \
NODE_AUTH_TOKEN="$(cat /run/secrets/npm_token)" npm install --no-audit --no-fund

COPY . .
ENV VITE_BASE_PATH=${BUILD_BASE_PATH}
RUN npm run build

# Runtime stage: tiny nginx serving the built SPA + healthz.
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/nginx.conf
COPY --from=build /app/dist /usr/share/nginx/html
COPY --from=build /app/public/healthz /usr/share/nginx/html/healthz
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
