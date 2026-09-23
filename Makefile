# GitPulse 开发与运维命令入口（在 Git Bash / Unix-like shell 下使用）

.PHONY: build run test vet fmt clean

build:        ## 编译二进制到 bin/
	go build -o bin/gitpulse ./cmd/gitpulse

run:          ## 本地运行一次（执行单次监控任务）
	go run ./cmd/gitpulse

test:         ## 运行全部测试
	go test ./...

vet:          ## 静态检查
	go vet ./...

fmt:          ## 格式化全部代码
	gofmt -w .

clean:        ## 清理构建产物
	rm -rf bin/
