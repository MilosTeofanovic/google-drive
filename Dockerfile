
FROM golang:1.26-alpine AS builder

WORKDIR /app


RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .


FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/server /app/server
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static


RUN mkdir -p /app/uploads

EXPOSE 8080

CMD ["/app/server"]