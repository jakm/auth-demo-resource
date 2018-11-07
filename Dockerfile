FROM golang:alpine AS build
ADD . /go/src/github.com/jakm/auth-demo-resource
RUN go install github.com/jakm/auth-demo-resource

FROM alpine
COPY --from=build /go/bin/auth-demo-resource /opt/auth-demo-resource/auth-demo-resource
ADD store /opt/auth-demo-resource/store
WORKDIR /opt/auth-demo-resource
CMD /opt/auth-demo-resource/auth-demo-resource
