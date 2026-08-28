FROM golang:1.27.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/event-hunter-api ./cmd/api \
 && CGO_ENABLED=0 go build -o /out/order-service ./cmd/order-service \
 && CGO_ENABLED=0 go build -o /out/payment-service ./cmd/payment-service \
 && CGO_ENABLED=0 go build -o /out/shipping-service ./cmd/shipping-service \
 && CGO_ENABLED=0 go build -o /out/event-lab ./cmd/event-lab \
 && CGO_ENABLED=0 go build -o /out/quality-worker ./cmd/quality-worker \
 && CGO_ENABLED=0 go build -o /out/technical-dlq-projector ./cmd/technical-dlq-projector

FROM alpine:3.24 AS runtime-base
RUN adduser -D -H eventhunter
USER eventhunter

FROM runtime-base AS runtime-api
COPY --from=build /out/event-hunter-api /usr/local/bin/event-hunter-api
ENTRYPOINT ["/usr/local/bin/event-hunter-api"]

FROM runtime-base AS runtime-order
COPY --from=build /out/order-service /usr/local/bin/order-service
ENTRYPOINT ["/usr/local/bin/order-service"]

FROM runtime-base AS runtime-payment
COPY --from=build /out/payment-service /usr/local/bin/payment-service
ENTRYPOINT ["/usr/local/bin/payment-service"]

FROM runtime-base AS runtime-shipping
COPY --from=build /out/shipping-service /usr/local/bin/shipping-service
ENTRYPOINT ["/usr/local/bin/shipping-service"]

FROM runtime-base AS runtime-event-lab
COPY --from=build /out/event-lab /usr/local/bin/event-lab
ENTRYPOINT ["/usr/local/bin/event-lab"]

FROM runtime-base AS runtime-quality-worker
COPY --from=build /out/quality-worker /usr/local/bin/quality-worker
ENTRYPOINT ["/usr/local/bin/quality-worker"]

FROM runtime-base AS runtime-technical-dlq-projector
COPY --from=build /out/technical-dlq-projector /usr/local/bin/technical-dlq-projector
ENTRYPOINT ["/usr/local/bin/technical-dlq-projector"]
