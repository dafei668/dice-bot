# 使用官方 Go 镜像作为构建环境
FROM golang:1.24-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的包
RUN apk add --no-cache gcc musl-dev sqlite-dev

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o dice-bot main.go

# 使用轻量级镜像作为运行环境
FROM alpine:latest

# 安装必要的包
RUN apk --no-cache add ca-certificates sqlite

# 设置工作目录
WORKDIR /root/

# 从构建阶段复制二进制文件
COPY --from=builder /app/dice-bot .

# 创建数据目录
RUN mkdir -p /data

# 设置环境变量
ENV DATABASE_URL=/data/dice_bot.db

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./dice-bot"]