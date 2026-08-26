# K8S 多集群管理平台 Makefile
#
# 使用说明：
#   - Linux / macOS / WSL：直接 `make <target>`
#   - Windows：安装 make 后用 Git Bash / WSL 运行（`choco install make` 或 `apt install make`）
#
# 常用：
#   make help         # 查看所有目标
#   make dev          # 一键启动开发环境（前端:3000 + API:8080 + Worker:8081）
#   make build        # 编译所有产物到 bin/ 与 dist/
#   make config       # 首次初始化 backend/configs/config.yaml

BACKEND_DIR  := backend
FRONTEND_DIR := frontend
BIN_DIR      := $(BACKEND_DIR)/bin
GO           := go
NPM          := npm
API_SERVER   := $(BIN_DIR)/api-server
TASK_WORKER  := $(BIN_DIR)/task-worker
GO_BUILD     := CGO_ENABLED=0 $(GO) build -ldflags "-s -w"

.DEFAULT_GOAL := help

.PHONY: help
help:  ## 显示所有可用目标
	@echo "K8S 多集群管理平台 Makefile"
	@echo ""
	@echo "可用目标："
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[32m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "常用组合："
	@echo "  \033[36mmake deps\033[0m         安装依赖（go mod + npm install）"
	@echo "  \033[36mmake config\033[0m        初始化 backend/configs/config.yaml"
	@echo "  \033[36mmake dev\033[0m           一键启动开发环境（3 个进程并行）"
	@echo "  \033[36mmake build\033[0m         编译所有产物"
	@echo "  \033[36mmake test\033[0m          运行测试与 lint"
	@echo "  \033[33mmake clean\033[0m         清理构建产物"

.PHONY: deps
deps: deps-backend deps-frontend  ## 安装所有依赖（go mod + npm install）

.PHONY: deps-backend
deps-backend:  ## 安装后端 Go 依赖
	@echo ">>> 安装后端依赖"
	cd $(BACKEND_DIR) && $(GO) mod download

.PHONY: deps-frontend
deps-frontend:  ## 安装前端 npm 依赖
	@echo ">>> 安装前端依赖"
	cd $(FRONTEND_DIR) && $(NPM) install

.PHONY: config
config:  ## 从示例生成 backend/configs/config.yaml
	@if [ ! -f $(BACKEND_DIR)/configs/config.yaml ]; then \
		cp $(BACKEND_DIR)/configs/config.example.yaml $(BACKEND_DIR)/configs/config.yaml; \
		echo ">>> 已生成 $(BACKEND_DIR)/configs/config.yaml（请按需修改 DB/Redis/JWT 等）"; \
	else \
		echo ">>> config.yaml 已存在，跳过"; \
	fi

.PHONY: build
build: build-backend build-frontend  ## 编译所有（后端二进制 + 前端 dist）

.PHONY: build-backend
build-backend:  ## 编译后端 api-server 和 task-worker 到 backend/bin/
	@echo ">>> 编译后端"
	@mkdir -p $(BIN_DIR)
	cd $(BACKEND_DIR) && $(GO_BUILD) -o bin/api-server  ./cmd/api-server
	cd $(BACKEND_DIR) && $(GO_BUILD) -o bin/task-worker ./cmd/task-worker
	@ls -lh $(BIN_DIR)/api-server $(BIN_DIR)/task-worker

.PHONY: build-frontend
build-frontend:  ## 编译前端到 frontend/dist
	@echo ">>> 编译前端"
	cd $(FRONTEND_DIR) && $(NPM) run build

.PHONY: dev
dev: config  ## 一键启动开发环境（前端 + API + Worker 并行，Ctrl+C 退出所有）
	@echo ">>> 启动开发环境：前端:3000 + API:8080 + Worker:8081"
	@echo ">>> 按 Ctrl+C 退出所有进程"
	@$(MAKE) -j 3 dev-frontend dev-api dev-worker

.PHONY: dev-frontend
dev-frontend:  ## 启动 Vite 前端开发服务器（:3000，自动代理 /api -> :8080）
	@echo ">>> [前端] Vite dev :3000"
	cd $(FRONTEND_DIR) && $(NPM) run dev

.PHONY: dev-api
dev-api: config  ## 启动 API Server（:8080）
	@echo ">>> [后端] API Server :8080"
	cd $(BACKEND_DIR) && $(GO) run ./cmd/api-server

.PHONY: dev-worker
dev-worker: config  ## 启动 Task Worker（:8081）
	@echo ">>> [后端] Task Worker :8081"
	cd $(BACKEND_DIR) && $(GO) run ./cmd/task-worker

.PHONY: run-prod
run-prod: build  ## 用编译产物运行生产模式（需先 make build）
	@echo ">>> 生产模式启动"
	@echo ">>> API Server :8080"
	@$(BIN_DIR)/api-server &
	@echo ">>> Task Worker :8081"
	@$(BIN_DIR)/task-worker &
	@wait

.PHONY: test
test: test-backend lint-frontend  ## 运行所有测试与 lint

.PHONY: test-backend
test-backend:  ## 运行后端 Go 测试（带竞态检测）
	@echo ">>> 运行后端测试"
	cd $(BACKEND_DIR) && $(GO) test -race -count=1 ./...

.PHONY: lint-frontend
lint-frontend:  ## 运行前端 eslint
	@echo ">>> 运行前端 lint"
	cd $(FRONTEND_DIR) && $(NPM) run lint

.PHONY: fmt
fmt:  ## 格式化 Go 代码
	cd $(BACKEND_DIR) && $(GO) fmt ./...

.PHONY: vet
vet:  ## 运行 go vet
	cd $(BACKEND_DIR) && $(GO) vet ./...

.PHONY: clean
clean:  ## 清理所有构建产物
	@echo ">>> 清理构建产物"
	rm -rf $(BIN_DIR)
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules/.vite
	@echo ">>> 已清理 bin/、dist/、.vite 缓存"
