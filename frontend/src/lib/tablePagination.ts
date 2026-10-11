import type { TablePaginationConfig } from 'antd'

type TFn = (key: string, options?: Record<string, unknown>) => string

/**
 * 全项目表格统一分页配置(I5):默认 10 条/页,可切换,显示总数。
 * 数据为全量拉取,分页仅在前端渲染层切片。
 */
export function tablePagination(t: TFn, defaultPageSize = 10): TablePaginationConfig {
  return {
    defaultPageSize,
    pageSizeOptions: [10, 20, 50, 100],
    showSizeChanger: true,
    showTotal: (total) => t('common.tableTotal', { total }),
  }
}
