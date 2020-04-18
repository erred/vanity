FROM golang:alpine AS build

WORKDIR /workspace
ENV CGO_ENABLED=0
COPY . .
RUN go build -o /bin/vanity


FROM scratch

COPY --from=build /bin/vanity /bin/

ENTRYPOINT [ "/bin/vanity" ]
