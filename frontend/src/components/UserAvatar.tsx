import { useEffect, useState } from 'react'

import { Avatar } from 'antd'

import { gravatarURL, sha256Hex } from '@/lib/gravatar'

export interface UserAvatarProps {
  username: string
  nickname?: string
  email?: string
  avatarURL?: string
  size?: number
}

/**
 * 头像三级回退(设计 §4.1):自定义上传 > Gravatar(镜像可配)> 首字母。
 * 每级加载失败自动降级,最终首字母,页面无破图。
 */
export function UserAvatar({ username, nickname, email, avatarURL, size = 28 }: UserAvatarProps) {
  const label = (nickname || username || '?').charAt(0).toUpperCase()
  // 降级游标:'upload → 'gravatar' → 'initial';src 加载失败时前移。
  const [stage, setStage] = useState<'upload' | 'gravatar' | 'initial'>(
    avatarURL ? 'upload' : 'gravatar',
  )
  const [hashInfo, setHashInfo] = useState<{ email: string; hash: string } | null>(null)

  // 派生 hash:email 未变沿用缓存,变了则视为未就绪(避免 effect 内同步 setState)。
  const hash = hashInfo && hashInfo.email === email ? hashInfo.hash : null
  const wantGravatar = !!email && stage !== 'initial'

  useEffect(() => {
    if (!email || !wantGravatar || hash) return
    const target = email
    let alive = true
    void sha256Hex(target).then((digest) => {
      if (alive) setHashInfo({ email: target, hash: digest })
    })
    return () => {
      alive = false
    }
  }, [email, wantGravatar, hash])

  if (stage === 'upload' && avatarURL) {
    return (
      <Avatar
        size={size}
        src={avatarURL}
        onError={() => {
          setStage('gravatar')
          return false
        }}
      />
    )
  }
  if (stage !== 'initial' && wantGravatar && hash) {
    return (
      <Avatar
        size={size}
        src={gravatarURL(hash, size)}
        onError={() => {
          setStage('initial')
          return false
        }}
      />
    )
  }
  return (
    <Avatar size={size} style={{ backgroundColor: '#1677ff', fontSize: size * 0.45 }}>
      {label}
    </Avatar>
  )
}
