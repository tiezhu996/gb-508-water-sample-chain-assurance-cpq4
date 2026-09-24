
const STATUS_LABELS: Record<string, string> = {
  received: '已接收', accepted: '已受理', testing: '在检', hold: '暂停', disposed: '已处置',
  draft: '草稿', validated: '已验证', active: '现行', retired: '已退役',
  peer_review: '待复核', signed: '已签发', rejected: '已驳回',
  planned: '计划中', collecting: '采样中', closed: '已关闭',
};

export function statusLabel(status: string | undefined | null): string {
  return (status && STATUS_LABELS[status]) || status || '-';
}

export function formatDate(value: string): string {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-';
}
export function nextStatus(current: string, statuses: readonly string[]): string | null {
  const index = statuses.indexOf(current);
  return index >= 0 && index < statuses.length - 1 ? statuses[index + 1] : null;
}
export function statusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (/approved|accepted|released|completed|signed|closed|pass|ready|online|cleared|succeeded/.test(status)) return 'success';
  if (/failed|rejected|critical|scrap|discard|revoked|urgent|disposed|retired/.test(status)) return 'danger';
  if (/hold|warning|review|pending|restricted|limited|quarantine/.test(status)) return 'warning';
  return 'neutral';
}
