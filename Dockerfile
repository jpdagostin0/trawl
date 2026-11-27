FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# cgo: DuckDB (vote/vector store) is a C++ library linked into the binary.
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /trawl .

# cc: glibc + libstdc++ for DuckDB
FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=build /trawl /trawl
ENV TRAWL_ADDR=:8088
EXPOSE 8088
ENTRYPOINT ["/trawl"]
