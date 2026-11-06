FROM golang:1.27.3-alpine AS build

ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/dengqi/telesend/internal/buildinfo.Version=${VERSION}" \
    -o /out/telesend ./

FROM gcr.io/distroless/static-debian13:nonroot

USER nonroot:nonroot
WORKDIR /app
COPY --from=build /out/telesend /app/telesend

ENTRYPOINT ["/app/telesend"]
