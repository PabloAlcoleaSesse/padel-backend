FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/padel-api .

FROM alpine:3.21

RUN adduser -D -H -s /sbin/nologin appuser

WORKDIR /app
COPY --from=build /bin/padel-api /app/padel-api

ENV PORT=8080
EXPOSE 8080

USER appuser

CMD ["/app/padel-api"]
