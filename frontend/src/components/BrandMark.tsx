/** 品牌圆环标识:登录页与主布局共用;颜色随传入 token 色自适应主题。 */
export function BrandMark({ size = 24, color }: { size?: number; color: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="8.5" stroke={color} strokeWidth="2.5" />
      <circle cx="12" cy="12" r="2.5" fill={color} />
    </svg>
  )
}
