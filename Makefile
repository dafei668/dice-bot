.PHONY: build test run clean docker-build docker-run docker-stop

# 构建应用
build:
	go build -o bin/telegram-dice-bot main.go

# 运行测试
test:
	go test ./test/... -v

# 运行应用
run:
	go run main.go

# 清理构建文件
clean:
	rm -rf bin/

# 构建Docker镜像
docker-build:
	docker build -t telegram-dice-bot .

# 运行Docker容器
docker-run:
	docker-compose up -d

# 停止Docker容器
docker-stop:
	docker-compose down

# 查看日志
docker-logs:
	docker-compose logs -f

# 重启服务
docker-restart: docker-stop docker-run

# 格式化代码
fmt:
	go fmt ./...

# 检查代码
lint:
	golangci-lint run

# 安装依赖
deps:
	go mod tidy
	go mod download

# 完整测试（包括构建和测试）
check: fmt build test

# 部署准备
deploy: clean build test docker-build

# 开发环境启动
dev: deps fmt build test run