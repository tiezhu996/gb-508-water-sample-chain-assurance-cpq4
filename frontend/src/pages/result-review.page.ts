
import { CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatInputModule } from '@angular/material/input';
import type { DomainRecord } from '../types/domain';
import { ResultReviewStore } from '../stores/result-review.store';
import { listLabSample } from '../api/lab-sample';
import { listAssayMethod } from '../api/assay-method';
import { authState, hasMinimumRole } from '../hooks/use-auth';
import { formatDate } from '../utils/format';
import { StatusBadgeComponent } from '../components/common/status-badge.component';
import { MetricCardComponent } from '../components/common/metric-card.component';
import { ConfirmDialogComponent } from '../components/common/confirm-dialog.component';
import { EmptyStateComponent } from '../components/common/empty-state.component';

const STATUS_LABELS: Record<string, string> = {
  received: '已接收', accepted: '已验收', testing: '在检', hold: '暂停', disposed: '已处置',
  draft: '草稿', validated: '已验证', active: '在用', retired: '已退役',
  peer_review: '复核中', signed: '已签发', rejected: '已驳回',
};

@Component({
  selector: 'app-result-review-page', standalone: true,
  imports: [CommonModule, FormsModule, MatButtonModule, MatInputModule, StatusBadgeComponent, MetricCardComponent, ConfirmDialogComponent, EmptyStateComponent],
  template: `
<main class="workspace" *ngIf="store.state$ | async as state">
  <header class="page-header"><div><p class="eyebrow">业务工作台</p><h1>结果复核</h1><p>复核单绑定在检样本与在用方法，提交复核时记录样本批次、状态与方法版本；签发前再次核对，样本不再适用或方法改版退役时拦截签发并留档。</p></div><button *ngIf="canWrite()" mat-flat-button color="primary" (click)="openCreate()">新增结果复核</button></header>
  <section class="metrics"><app-metric-card label="记录总数" [value]="state.meta.total" detail="当前筛选范围"/><app-metric-card label="待签发" [value]="countByStatus(state.items, 'peer_review')" detail="等待复核员签发"/><app-metric-card label="被拦截留档" [value]="blockedCount(state.items)" detail="旧单留档不签发，需新建复核单"/></section>
  <section class="toolbar"><input matInput aria-label="搜索" [(ngModel)]="search" placeholder="搜索结果复核编码或名称"/><button mat-flat-button color="primary" (click)="query()">查询</button><button mat-button (click)="reset()">重置</button></section>
  <div *ngIf="state.error" class="alert">{{ state.error }}</div>
  <section class="table-shell"><table><thead><tr><th>编码</th><th>复核依据（提交时快照）</th><th>当前状态核对</th><th>状态</th><th>提示</th><th>操作</th></tr></thead><tbody>
    <tr *ngFor="let item of state.items">
      <td><strong>{{ item.code }}</strong><small>{{ item.name }}</small></td>
      <td><div class="basis"><span>样本 {{ item.sampleCode || '-' }} · {{ labelOf(item.sampleStatus) || '未快照' }}</span><small>批次 {{ item.sampleBatchCode || '-' }} · 方法 {{ item.methodCode || '-' }}<span *ngIf="item.methodVersion"> v{{ item.methodVersion }}</span></small></div></td>
      <td><div class="basis"><span>样本 <app-status-badge *ngIf="sampleOf(item) as sample" [status]="sample.status"/><span *ngIf="!sampleOf(item)" class="muted">已删除</span></span><small>方法 <ng-container *ngIf="methodOf(item) as method">v{{ method.version }} <app-status-badge [status]="method.status"/></ng-container><span *ngIf="!methodOf(item)" class="muted">已删除</span></small></div></td>
      <td><app-status-badge [status]="item.status"/></td>
      <td>
        <span *ngIf="item.blockedReason" class="flag flag--danger">已拦截：{{ item.blockedReason }}</span>
        <ng-container *ngIf="!item.blockedReason">
          <span *ngFor="let warning of staleWarnings(item)" class="flag flag--warning">{{ warning }}</span>
          <span *ngIf="!staleWarnings(item).length" class="flag flag--muted">依据一致</span>
        </ng-container>
      </td>
      <td>
        <span *ngIf="item.blockedReason" class="muted">留档不签发</span>
        <div *ngIf="!item.blockedReason" class="action-group">
          <button *ngIf="item.status === 'draft' && canWrite()" class="table-action" (click)="openTransition(item, 'peer_review')">提交复核</button>
          <button *ngIf="item.status === 'peer_review' && canReview()" class="table-action" (click)="openTransition(item, 'signed')">签发</button>
          <button *ngIf="item.status === 'peer_review' && canReview()" class="table-action" (click)="openTransition(item, 'rejected')">驳回</button>
          <button *ngIf="item.status === 'signed' && canReview()" class="table-action" (click)="openTransition(item, 'rejected')">作废</button>
          <button *ngIf="item.status === 'rejected' && canWrite()" class="table-action" (click)="openTransition(item, 'peer_review')">重新提交</button>
        </div>
      </td>
    </tr>
    <tr *ngIf="!state.items.length && !state.loading"><td colspan="6"><app-empty-state title="暂无结果复核"/></td></tr>
  </tbody></table><div *ngIf="state.loading" class="loading">正在同步业务数据…</div></section>
  <div *ngIf="showCreate" class="modal-backdrop"><section class="modal" role="dialog"><h2>新增结果复核</h2>
    <label class="form-field"><span>在检样本</span><select [(ngModel)]="createForm.sampleCode" aria-label="在检样本"><option value="">选择在检样本</option><option *ngFor="let sample of samples" [value]="sample.code" [disabled]="sample.status !== 'testing'">{{ sample.code }} · {{ sample.name }}（{{ labelOf(sample.status) }}）</option></select></label>
    <label class="form-field"><span>检测方法</span><select [(ngModel)]="createForm.methodCode" aria-label="检测方法"><option value="">选择使用的方法</option><option *ngFor="let method of methods" [value]="method.code" [disabled]="method.status !== 'active'">{{ method.code }} · {{ method.name }}（v{{ method.version }} {{ labelOf(method.status) }}）</option></select></label>
    <label class="form-field"><span>复核说明</span><input matInput [(ngModel)]="createForm.description" placeholder="选填，记录复核背景"/></label>
    <p class="muted">提交复核时将记录样本批次、状态与方法版本作为签发依据。</p>
    <footer><button mat-button (click)="closeCreate()">取消</button><button mat-flat-button color="primary" [disabled]="!createForm.sampleCode || !createForm.methodCode" (click)="create()">创建复核单</button></footer>
  </section></div>
  <app-confirm-dialog [open]="!!pending" [title]="pending?.status === 'signed' ? '确认签发' : '确认状态迁移'" (cancel)="closeTransition()" (confirm)="confirmTransition()">
    <p *ngIf="pending?.status === 'signed'">签发前将再次核对样本状态与方法版本，样本不再适用或方法改版退役时会拦截签发并写入审核记录。</p>
    <p *ngIf="pending?.status !== 'signed'">状态迁移会写入审计日志，并使用版本号避免并发覆盖。</p>
    <strong>{{ pending?.item?.status }} → {{ pending?.status }}</strong>
  </app-confirm-dialog>
</main>` })
export class ResultReviewPage implements OnInit {
  search = '';
  showCreate = false;
  samples: DomainRecord[] = [];
  methods: DomainRecord[] = [];
  sampleMap = new Map<string, DomainRecord>();
  methodMap = new Map<string, DomainRecord>();
  createForm = { sampleCode: '', methodCode: '', description: '' };
  pending: { item: DomainRecord; status: string } | null = null;
  readonly formatDate = formatDate;
  readonly auth = authState;
  constructor(readonly store: ResultReviewStore, private readonly changeDetector: ChangeDetectorRef) {}
  async ngOnInit() { await Promise.all([this.load(), this.loadRefs()]); }
  async query() { await this.load(this.search); }
  async reset() { this.search = ''; await this.load(); }
  canWrite() { return hasMinimumRole(this.auth.session()?.role, 'operator'); }
  canReview() { return hasMinimumRole(this.auth.session()?.role, 'reviewer'); }
  countByStatus(items: DomainRecord[], status: string) { return items.filter((item) => item.status === status).length; }
  blockedCount(items: DomainRecord[]) { return items.filter((item) => !!item.blockedReason).length; }
  labelOf(status?: string) { return status ? STATUS_LABELS[status] || status : ''; }
  sampleOf(item: DomainRecord) { return item.sampleCode ? this.sampleMap.get(item.sampleCode) : undefined; }
  methodOf(item: DomainRecord) { return item.methodCode ? this.methodMap.get(item.methodCode) : undefined; }
  staleWarnings(item: DomainRecord): string[] {
    const warnings: string[] = [];
    if (item.sampleCode) {
      const sample = this.sampleMap.get(item.sampleCode);
      if (!sample) warnings.push(`样本 ${item.sampleCode} 已删除`);
      else if (item.sampleStatus && sample.status !== item.sampleStatus) warnings.push(`样本状态已变为${this.labelOf(sample.status)}`);
      else if (!item.sampleStatus && sample.status !== 'testing') warnings.push(`样本当前${this.labelOf(sample.status)}，不在检`);
    }
    if (item.methodCode) {
      const method = this.methodMap.get(item.methodCode);
      if (!method) warnings.push(`方法 ${item.methodCode} 已删除`);
      else if (method.status === 'retired') warnings.push('方法已退役');
      else if (item.methodVersion && method.version !== item.methodVersion) warnings.push(`方法已换版 v${item.methodVersion} → v${method.version}`);
      else if (!item.methodVersion && method.status !== 'active') warnings.push('方法当前非在用');
    }
    return warnings;
  }
  async openCreate() { await this.loadRefs(); this.showCreate = true; this.changeDetector.detectChanges(); }
  closeCreate() { this.showCreate = false; this.changeDetector.detectChanges(); }
  openTransition(item: DomainRecord, status: string) { this.pending = { item, status }; this.changeDetector.detectChanges(); }
  closeTransition() { this.pending = null; this.changeDetector.detectChanges(); }
  async create() {
    const sample = this.sampleMap.get(this.createForm.sampleCode);
    if (!sample) return;
    try {
      await this.store.createRecord('reviews', {
        code: `RR-${String(Date.now()).slice(-8)}`, name: `${sample.name}复核`,
        description: this.createForm.description || '复核单创建', facility: sample.facility || '默认作业区',
        owner: this.auth.session()?.username || 'operator', category: sample.category || '常规',
        riskLevel: sample.riskLevel || 'medium', metricValue: sample.metricValue ?? 0, metricUnit: sample.metricUnit || 'unit',
        effectiveAt: new Date().toISOString(), evidence: `依据样本 ${sample.code} 与方法 ${this.createForm.methodCode} 创建`,
        relatedCode: sample.relatedCode || '', sampleCode: this.createForm.sampleCode, methodCode: this.createForm.methodCode,
      });
      this.createForm = { sampleCode: '', methodCode: '', description: '' };
      this.showCreate = false;
    } catch { /* Store exposes the request error in its observable state. */ } finally { this.changeDetector.detectChanges(); }
  }
  async confirmTransition() {
    if (!this.pending) return;
    try { await this.store.transition('reviews', this.pending.item, this.pending.status); this.pending = null; }
    catch { await this.store.load('reviews', this.search); /* Refresh so an archived block becomes visible. */ }
    finally { await this.loadRefs(); this.changeDetector.detectChanges(); }
  }
  private async load(search = '') { await this.store.load('reviews', search); this.changeDetector.detectChanges(); }
  private async loadRefs() {
    try {
      const [samples, methods] = await Promise.all([listLabSample(1, 100), listAssayMethod(1, 100)]);
      this.samples = samples.data; this.methods = methods.data;
      this.sampleMap = new Map(samples.data.map((item) => [item.code, item]));
      this.methodMap = new Map(methods.data.map((item) => [item.code, item]));
    } catch { /* Reference lists stay empty and rows fall back to snapshot data. */ }
  }
}
