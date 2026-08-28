FROM node:26.7-alpine AS build
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN npm install --global pnpm@11.24.0 \
 && pnpm install --frozen-lockfile
COPY . .
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
ARG VITE_GRAFANA_URL=http://localhost:28332
ENV VITE_GRAFANA_URL=${VITE_GRAFANA_URL}
RUN pnpm build

FROM nginx:1.30.4-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
