# SPDX-License-Identifier: AGPL-3.0-only
# web/dist is built here and embedded by web/embed.go (-tags webembed), which the UI package owns.

FROM node:24 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
# VERSION is the calendar coordinate without the leading v, injected as
# -X github.com/inspr-at/aeon/internal/version.Version=${VERSION}
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags webembed \
    -ldflags "-X github.com/inspr-at/aeon/internal/version.Version=${VERSION}" \
    -o /aeon ./cmd/aeon

FROM gcr.io/distroless/static:nonroot
COPY --from=build /aeon /aeon
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/aeon", "serve"]
