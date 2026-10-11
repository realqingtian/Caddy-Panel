# 带插件的 Caddy 镜像:限流(caddy-rate-limit)、DNS challenge(cloudflare)、
# l4 转发(caddy-l4,T3.4)。
# 面板渲染对应指令(FR-704 限流 / FR-703 DNS challenge / l4 站点)需要此镜像;
# 不需要插件时 compose 可直接用官方 caddy:2 镜像。
#
# 多架构性能关键:xcaddy 编译阶段固定在构建机原生平台(BUILDPLATFORM),
# 通过 GOOS/GOARCH 环境变量交叉编译目标架构(Caddy 与插件均为纯 Go),
# 避免 arm64 在 QEMU 模拟下编译超慢;运行时层使用目标平台官方镜像。
FROM --platform=$BUILDPLATFORM caddy:2-builder AS builder
ARG TARGETOS
ARG TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH xcaddy build \
  --with github.com/mholt/caddy-ratelimit \
  --with github.com/caddy-dns/cloudflare \
  --with github.com/mholt/caddy-l4

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
