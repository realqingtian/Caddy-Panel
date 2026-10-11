/** Gravatar 镜像缓存(设计 §4.1):登录后拉取 settings 注入;默认官方源。 */
let mirrorBase = 'https://gravatar.com'

/** 注入镜像 base(应用启动拉取 settings 后调用)。 */
export function setGravatarMirror(mirror: string) {
  if (mirror.trim()) {
    mirrorBase = mirror.trim().replace(/\/+$/, '')
  } else {
    mirrorBase = 'https://gravatar.com'
  }
}

/** 按镜像 base 拼头像 URL(hash 为 email 归一化 SHA-256);hash 异步,调用方缓存。 */
export function gravatarURL(hash: string, size: number): string {
  return `${mirrorBase}/avatar/${hash}?s=${size * 2}&d=retro`
}

/** email 归一化 SHA-256(Gravatar 新标准,避免 MD5 可逆推)。 */
export async function sha256Hex(email: string): Promise<string> {
  const digest = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(email.trim().toLowerCase()),
  )
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}
