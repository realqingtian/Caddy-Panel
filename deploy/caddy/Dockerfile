# 带插件的 Caddy 镜像:限流(caddy-rate-limit)与 DNS challenge(cloudflare)。
# 面板渲染对应指令(FR-704 限流 / FR-703 DNS challenge)需要此镜像;
# 不需要插件时 compose 可直接用官方 caddy:2 镜像。
FROM caddy:2-builder AS builder
RUN xcaddy build \
  --with github.com/mholt/caddy-rate-limit \
  --with github.com/caddy-dns/cloudflare

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
