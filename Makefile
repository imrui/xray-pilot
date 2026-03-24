.PHONY: all frontend build dev-backend dev-frontend clean

# 默认目标
all: build

# 编译前端
frontend:
	cd frontend && npm run build

# 完整构建（前端 + Go 二进制）
build: frontend
	go build -o xray-pilot .

# 仅启动后端开发服务器
dev-backend:
	go run main.go

# 仅启动前端开发服务器
dev-frontend:
	cd frontend && npm run dev

# 清理构建产物
clean:
	rm -f xray-pilot xray-pilot.db
	rm -rf frontend/dist
