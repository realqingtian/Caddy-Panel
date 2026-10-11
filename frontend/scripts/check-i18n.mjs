// i18n 静态审查:扫描 src 下所有 t('...') / t("...") 调用,与两份语言文件对比
// 输出:1) 代码使用了但语言文件缺失的 key 2) zh-CN 与 en-US 结构不对齐的 key 3) 疑似动态 key
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

const SRC = new URL('../src', import.meta.url).pathname

function flat(obj, prefix = '', out = new Map()) {
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') flat(v, path, out)
    else out.set(path, v)
  }
  return out
}

const zh = flat(JSON.parse(readFileSync(join(SRC, 'locales/zh-CN.json'), 'utf8')))
const en = flat(JSON.parse(readFileSync(join(SRC, 'locales/en-US.json'), 'utf8')))

function walk(dir, files = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, files)
    else if (/\.(tsx?|jsx?)$/.test(name)) files.push(p)
  }
  return files
}

const staticKeys = new Set()
const dynamicCalls = []
const hardcoded = []

const T_CALL = /\bt\(\s*(['"])((?:(?!\1)[^\\])*)\1/g
const STRING_LITERAL_CN = /['"`]([\u4e00-\u9fff][^'"`\n]*)['"`]/g

for (const file of walk(SRC)) {
  if (file.includes('locales/')) continue
  const code = readFileSync(file, 'utf8')
  for (const m of code.matchAll(T_CALL)) staticKeys.add(m[2])
  for (const m of code.matchAll(/\bt\(\s*`([^`]+)`/g)) dynamicCalls.push(`${file}: ${m[1]}`)
  // 硬编码中文(排除注释行)
  const lines = code.split('\n')
  lines.forEach((line, i) => {
    const noComment = line.replace(/\/\/.*$/, '').replace(/\{\/\*.*?\*\/\}/g, '')
    if (new RegExp(STRING_LITERAL_CN.source).test(noComment)) {
      hardcoded.push(`${file.replace(SRC, 'src')}:${i + 1}: ${line.trim().slice(0, 120)}`)
    }
  })
}

const missingInZh = [...staticKeys].filter((k) => !zh.has(k))
const missingInEn = [...staticKeys].filter((k) => !en.has(k))
const zhOnly = [...zh.keys()].filter((k) => !en.has(k))
const enOnly = [...en.keys()].filter((k) => !zh.has(k))
const unused = [...zh.keys()].filter((k) => !staticKeys.has(k) && !dynamicCalls.some((d) => k.startsWith(d.replace(/\$\{[^}]*\}/g, ''))))

console.log('=== 代码使用但 zh-CN 缺失(会裸露 key)===')
missingInZh.forEach((k) => console.log('  ' + k))
console.log('=== 代码使用但 en-US 缺失(切英文会裸露 key)===')
missingInEn.forEach((k) => console.log('  ' + k))
console.log('=== 仅 zh-CN 有(en-US 缺,反向不对齐)===')
zhOnly.forEach((k) => console.log('  ' + k))
console.log('=== 仅 en-US 有 ===')
enOnly.forEach((k) => console.log('  ' + k))
console.log('=== 疑似动态 key(需人工核对)===')
dynamicCalls.forEach((k) => console.log('  ' + k))
console.log('=== 疑似硬编码中文(需人工核对)===')
hardcoded.forEach((k) => console.log('  ' + k))
console.log('=== 语言文件中未被静态扫描引用(可能动态使用,仅提示)===')
console.log('  共 ' + unused.length + ' 个;' + unused.slice(0, 40).join(', '))
console.log(`\n统计: 静态 key ${staticKeys.size} 个 | zh ${zh.size} | en ${en.size}`)
