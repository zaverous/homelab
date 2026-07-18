# syntax=docker/dockerfile:1
# Multi-stage: node builds the static bundle, nginx serves it and proxies /api.

FROM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html vite.config.ts tsconfig.json ./
COPY src ./src
RUN npm run build

FROM nginx:1.27-alpine
# Default upstream = the in-cluster API Service; the nginx entrypoint's
# envsubst renders the template with this (or an override) at startup.
ENV API_UPSTREAM=kubepets-api.kubepets.svc.cluster.local:80
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
