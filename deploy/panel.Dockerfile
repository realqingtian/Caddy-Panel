# Caddy Panel 后端镜像:多阶段构建,最终产物为"单二进制 + 内嵌前端"。
# 构建上下文为仓库根目录(compose 中 context: ..)。

# ---------- 阶段 1:前端(Bun 只做包管理与脚本执行,构建走 Vite) ----------
FROM oven/bun:1 AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
# 产物写入后端 embed 目录 ../backend/internal/web/dist
RUN bun run build

# ---------- 阶段 2:后端(纯 Go SQLite 驱动,CGO_ENABLED=0 静态编译) ----------
FROM golang:1.27-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# swag 生成物不入库(gitignore 约定),镜像内重新生成;前端产物就地覆盖 embed 目录
COPY --from=frontend /app/backend/internal/web/dist ./internal/web/dist
RUN go install github.com/swaggo/swag/cmd/swag@latest \
  && swag init -g cmd/panel/main.go -o docs_swag \
  && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/panel ./cmd/panel

# ---------- 阶段 3:运行时 ----------
FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S panel && adduser -S panel -G panel \
  && mkdir -p /data && chown -R panel:panel /data
COPY --from=backend /out/panel /usr/local/bin/panel
USER panel
EXPOSE 8080
ENTRYPOINT ["panel"]
