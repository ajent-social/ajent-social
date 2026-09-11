FROM golang:1.26.6-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG AJENT_REVISION=unknown
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-X main.buildRevision=${AJENT_REVISION}" -o /out/ajentd ./cmd/ajentd && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/aj ./cmd/aj
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ /app/
# GA4's measurement ID is public configuration, not a credential. Override this
# environment variable in a deployment that should not collect web analytics.
ENV AJENT_GA_MEASUREMENT_ID=G-B4WXNXM8HQ
EXPOSE 8080
ENTRYPOINT ["/app/ajentd"]
CMD ["-addr", "0.0.0.0:8080"]
