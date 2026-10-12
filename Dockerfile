# 基础镜像前缀：默认留空 = 走默认源；CI 里通过 build-arg 传入私有镜像仓库的
# 代理前缀（形如 <registry>/dockerhub/），仓库里不写死任何内网地址
ARG REGISTRY_PREFIX=

# 构建阶段 (也作为开发环境)
FROM ${REGISTRY_PREFIX}library/golang:1.26-alpine AS builder
WORKDIR /app

RUN go env -w GOPROXY=https://goproxy.cn,direct
# 1. 在有 Go 环境的这一层安装 air
RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod\
    go build -o main ./cmd/

# Runtime image shared by Compose and Kubernetes
FROM ${REGISTRY_PREFIX}library/alpine:latest AS runtime
WORKDIR /app
# 安装时区数据
RUN apk add --no-cache tzdata
ENV TZ=Asia/Shanghai

COPY --from=builder /app/main .

CMD ["./main"]
