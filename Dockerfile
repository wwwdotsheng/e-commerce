# 构建阶段 (也作为开发环境)
FROM harbor.local/dockerhub/golang:1.25.6-alpine AS builder
WORKDIR /app

RUN go env -w GOPROXY=https://goproxy.cn,direct
# 1. 在有 Go 环境的这一层安装 air
RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main ./cmd/

# 运行阶段 (仅用于生产环境打包)
FROM harbor.local/dockerhub/alpine:latest
WORKDIR /app
# 安装时区数据
RUN apk add --no-cache tzdata
ENV TZ=Asia/Shanghai

COPY --from=builder /app/main .
COPY --from=builder /go/bin/air /usr/local/bin/air

CMD ["./main"]