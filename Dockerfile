FROM golang:alpine AS build

WORKDIR /workspace
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /bin/vanity


FROM scratch

COPY --from=build /bin/vanity /bin/

ENTRYPOINT [ "/bin/vanity" ]
