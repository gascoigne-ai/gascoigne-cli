FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gascoigne ./cmd/gascoigne

FROM alpine:3
RUN apk add --no-cache ca-certificates && adduser -D gascoigne
USER gascoigne
COPY --from=build /gascoigne /usr/local/bin/gascoigne
ENTRYPOINT ["gascoigne"]
