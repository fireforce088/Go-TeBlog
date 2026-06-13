FROM golang:1.25-alpine AS builder

WORKDIR /src

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/blog_app ./main.go ./skin.go ./editor_normalize.go ./image_localizer.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/admin_app ./admin.go ./admin_helpers.go ./admin_storage.go ./skin.go ./editor_normalize.go ./image_localizer.go

FROM alpine:3.22

RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apk/repositories && \
    apk add --no-cache bash ca-certificates curl sqlite tar tzdata

WORKDIR /app

COPY --from=builder /out/blog_app /app/blog_app
COPY --from=builder /out/admin_app /app/admin_app
COPY templates /app/templates
COPY usr /app/usr
COPY static /app/static
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY backup.sh /usr/local/bin/backup.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/backup.sh

ENV TZ=Asia/Shanghai \
    INIT_ADMIN_USER=admin \
    INIT_ADMIN_PASSWORD=

VOLUME ["/data"]
EXPOSE 8190 8191

ENTRYPOINT ["docker-entrypoint.sh"]
