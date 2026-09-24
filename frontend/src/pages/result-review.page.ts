import { CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatInputModule } from '@angular/material/input';
import { ResultReviewStore } from '../stores/result-review.store';
import { authState, hasMinimumRole } from '../hooks/use-auth';
import { formatDate, statusLabel } from '../utils/format';
import type { CreateReviewInput, DomainRecord } from '../types/domain';
import { StatusBadgeComponent } from '../components/common/status-badge.component';
import { MetricCardComponent } from '../components/common/metric-card.component';
import { ConfirmDialogComponent } from '../components/common/confirm-dialog.component';
import { EmptyStateComponent } from '../components/common/empty-state.component';

@Component({
  selector: 'app-result-review-page',
  standalone: true,
  imports: [CommonModule, FormsModule, MatButtonModule, MatInputModule, StatusBadgeComponent, MetricCardComponent, ConfirmDialogComponent, EmptyStateComponent],
  template: `
<main class="workspace" *ngIf="store.state$ | async as state">
  <header class="page-header">
    <div>
      <p class="eyebrow">质量签发工作台</p>
      <h1>结果复核</h1>
      <p>提交复核时冻结样本批次、状态与方法版本；签发前再次核对，样本暂停/处置或方法换版退役将被拦截，旧单留档不可签发。</p>
    </div>
    <button *ngIf="canWrite()" mat-flat-button color="primary" (click)="openCreate()">新增复核单</button>
  </header>

  <section class="metrics">
    <app-metric-card label="复核单总数" [value]="state.meta.total" detail="当前筛选范围"/>
    <app-metric-card label="待复核" [value]="pendingCount(state.items)" detail="等待签发核对"/>
    <app-metric-card label="依据过期被挡" [value]="blockedCount(state.items)" detail="旧单留档，需补正新建"/>
  </section>

  <section class="toolbar">
    <input matInput aria-label="搜索" [(ngModel)]="search" placeholder="搜索复核单编码或名称"/>
    <button mat-flat-button color="primary" (click)="query()">查询</button>
    <button mat-button (click)="reset()">重置</button>
  </section>
  <div *ngIf="state.error" class="alert">{{ state.error }}</div>

  <section class="table-shell">
    <table class="review-table">
      <thead><tr>
        <th>复核单</th><th>复核依据（提交时冻结）</th><th>签发前核对（当前状态）</th><th>复核状态</th><th>提交/签发</th><th>更新时间</th><th>操作</th>
      </tr></thead>
      <tbody>
        <tr *ngFor="let item of state.items" [class.row--blocked]="isBlocked(item)">
          <td style="min-width:170px">
            <strong>{{ item.code }}</strong>
            <small>{{ item.name }}</small>
            <small>{{ item.facility }} · {{ item.metricValue }} {{ item.metricUnit }}</small>
          </td>
          <td style="min-width:230px">
            <ng-container *ngIf="item.sampleCode; else noBasis">
              <small><strong>样本</strong> {{ item.sampleCode }} · 批次 {{ item.sampleBatch || '-' }}</small>
              <small>提交时状态：{{ statusLabel(item.sampleSnapshot) }}</small>
              <small><strong>方法</strong> {{ item.methodCode }} · v{{ item.methodSnapshot }}（{{ statusLabel(item.methodStatus) }}）</small>
            </ng-container>
            <ng-template #noBasis><small class="muted">尚未提交复核，依据待冻结</small></ng-template>
          </td>
          <td style="min-width:230px">
            <ng-container *ngIf="item.basis as basis; else basisLoading">
              <span class="basis-pill" [class.basis-pill--ok]="basis.eligible && !basis.locked && item.status === 'peer_review'"
                    [class.basis-pill--bad]="(!basis.eligible || basis.locked) && item.status === 'peer_review'"
                    [class.basis-pill--idle]="item.status !== 'peer_review'">
                {{ basisText(item) }}
              </span>
              <small>样本当前：<strong [class.stale]="basis.sampleMissing || basis.sampleStatus !== 'testing'">{{ basis.sampleMissing ? '不存在' : statusLabel(basis.sampleStatus) }}</strong></small>
              <small>方法当前：<strong [class.stale]="basis.methodMissing || basis.methodStatus !== 'active' || basis.methodVersion !== item.methodSnapshot">
                {{ basis.methodMissing ? '不存在' : ('v' + basis.methodVersion + ' · ' + statusLabel(basis.methodStatus)) }}
              </strong></small>
              <div *ngIf="isBlocked(item)" class="block-reasons">
                <strong>被挡原因：</strong>
                <ul><li *ngFor="let reason of blockReasons(item)">{{ reason }}</li></ul>
                <em>旧单留档不可签发，补正后请新建复核单。</em>
              </div>
            </ng-container>
            <ng-template #basisLoading><small class="muted">正在核对…</small></ng-template>
          </td>
          <td><app-status-badge [status]="item.status"/></td>
          <td style="min-width:120px">
            <small>提交：{{ item.reviewRequestedBy || '-' }}</small>
            <small>签发：{{ item.signedBy || '-' }}</small>
          </td>
          <td>{{ formatDate(item.updatedAt) }}</td>
          <td style="min-width:150px">
            <button *ngIf="canSubmit(item)" class="table-action" (click)="confirmSubmit(item)">提交复核</button>
            <ng-container *ngIf="item.status === 'peer_review'">
              <button class="table-action" [disabled]="!canSign(item)" [title]="signTitle(item)" (click)="confirmSign(item)">签发</button>
              <button class="table-action table-action--muted" *ngIf="canReview()" [disabled]="isBlocked(item)" [title]="isBlocked(item) ? '留档旧单不可驳回，只能新建复核单' : ''" (click)="confirmReject(item)">驳回</button>
              <small *ngIf="isBlocked(item)" class="blocked-tag">已拦截留档</small>
            </ng-container>
            <span *ngIf="item.status === 'signed'" class="muted">流程结束</span>
            <span *ngIf="item.status === 'rejected'" class="muted">已驳回</span>
          </td>
        </tr>
        <tr *ngIf="!state.items.length && !state.loading"><td colspan="7"><app-empty-state title="暂无结果复核单"/></td></tr>
      </tbody>
    </table>
    <div *ngIf="state.loading" class="loading">正在同步复核数据…</div>
  </section>

  <app-confirm-dialog [open]="showCreate" [title]="'新增复核单（读取最新样本与方法）'" [confirmDisabled]="!formReady()" (cancel)="closeCreate()" (confirm)="createReview()">
    <div class="review-form">
      <p class="form-hint">仅可选择<strong>在检样本</strong>与<strong>现行方法</strong>；提交复核时系统会重新读取并冻结批次、状态与方法版本。</p>
      <label>复核单名称<input matInput [(ngModel)]="form.name" placeholder="例如：总磷结果复核"/></label>
      <div class="form-grid">
        <label>作业区域<input matInput [(ngModel)]="form.facility" placeholder="作业区域"/></label>
        <label>责任人<input matInput [(ngModel)]="form.owner" placeholder="责任人"/></label>
      </div>
      <div class="form-grid">
        <label>类别<input matInput [(ngModel)]="form.category" placeholder="类别"/></label>
        <label>风险等级
          <select [(ngModel)]="form.riskLevel">
            <option value="low">low</option><option value="medium">medium</option>
            <option value="high">high</option><option value="critical">critical</option>
          </select>
        </label>
      </div>
      <div class="form-grid">
        <label>指标值<input matInput type="number" [(ngModel)]="form.metricValue"/></label>
        <label>单位<input matInput [(ngModel)]="form.metricUnit" placeholder="mg/L"/></label>
      </div>
      <label>在检样本
        <select [(ngModel)]="form.sampleId" (ngModelChange)="onSampleChange()">
          <option [ngValue]="0" disabled>选择在检样本</option>
          <option *ngFor="let sample of references.samples" [ngValue]="sample.id">
            {{ sample.code }} · {{ sample.name }}（批次 {{ sample.relatedCode || '-' }}，v{{ sample.version }}）
          </option>
        </select>
      </label>
      <small *ngIf="selectedSample()" class="form-hint">样本批次：{{ selectedSample()?.relatedCode || '-' }}，当前状态：{{ statusLabel(selectedSample()?.status) }}</small>
      <label>使用的方法（现行版本）
        <select [(ngModel)]="form.methodId">
          <option [ngValue]="0" disabled>选择现行方法</option>
          <option *ngFor="let method of references.methods" [ngValue]="method.id">
            {{ method.code }} · {{ method.name }}（v{{ method.version }}，{{ statusLabel(method.status) }}）
          </option>
        </select>
      </label>
      <label>检测证据<input matInput [(ngModel)]="form.evidence" placeholder="证据/质控说明"/></label>
      <div *ngIf="references.error" class="alert">{{ references.error }}</div>
      <small *ngIf="!references.samples.length || !references.methods.length" class="form-hint">没有可选的在检样本或现行方法时，请先在对应页面推进状态。</small>
    </div>
  </app-confirm-dialog>

  <app-confirm-dialog [open]="!!pending" [title]="pending?.title || '确认操作'" (cancel)="closePending()" (confirm)="runPending()">
    <p>{{ pending?.message }}</p>
    <strong *ngIf="pending?.item as p">{{ p.code }}：{{ p.status }} → {{ pending?.target }}</strong>
  </app-confirm-dialog>
</main>
  `,
})
export class ResultReviewPage implements OnInit {
  readonly auth = authState;
  readonly formatDate = formatDate;
  readonly statusLabel = statusLabel;
  search = '';
  showCreate = false;
  references = { samples: [] as DomainRecord[], methods: [] as DomainRecord[], loading: false, error: '' };
  form = this.emptyForm();
  pending: { item: DomainRecord; target: string; title: string; message: string } | null = null;

  constructor(readonly store: ResultReviewStore, private readonly changeDetector: ChangeDetectorRef) {}

  async ngOnInit(): Promise<void> {
    this.store.references$.subscribe((value) => { this.references = value; this.changeDetector.detectChanges(); });
    await Promise.all([this.load(), this.store.loadReferences()]);
  }

  private emptyForm(): CreateReviewInput {
    return {
      code: '', name: '', description: '前端工作台创建的复核单', facility: '', owner: this.auth.session()?.displayName || '',
      category: '常规', riskLevel: 'medium', metricValue: 0, metricUnit: 'mg/L',
      effectiveAt: new Date().toISOString(), evidence: '', sampleId: 0, methodId: 0,
    };
  }

  async load(search = ''): Promise<void> {
    await this.store.load('reviews', search);
    this.changeDetector.detectChanges();
  }
  async query(): Promise<void> { await this.load(this.search); }
  async reset(): Promise<void> { this.search = ''; await this.load(); }

  private syncReferences(): void {
    this.references = this.store.referenceSnapshot;
  }

  canWrite(): boolean { return hasMinimumRole(this.auth.session()?.role, 'operator'); }
  canReview(): boolean { return hasMinimumRole(this.auth.session()?.role, 'reviewer'); }
  pendingCount(items: DomainRecord[]): number { return items.filter((item) => item.status === 'peer_review').length; }
  blockedCount(items: DomainRecord[]): number { return items.filter((item) => this.isBlocked(item)).length; }

  isBlocked(item: DomainRecord): boolean {
    return item.status === 'peer_review' && !!item.basis && (item.basis.locked || !item.basis.eligible);
  }
  blockReasons(item: DomainRecord): string[] {
    if (!item.basis) return [];
    if (item.basis.locked && item.basis.lockedReason) return [item.basis.lockedReason, ...item.basis.reasons.filter((reason) => reason !== item.basis?.lockedReason)];
    return item.basis.reasons;
  }
  basisText(item: DomainRecord): string {
    if (item.status !== 'peer_review') return '无需核对';
    if (!item.basis) return '核对中';
    if (this.isBlocked(item)) return item.basis.locked ? '依据过期 · 旧单已留档' : '依据已过期';
    return '依据有效';
  }

  selectedSample(): DomainRecord | undefined { return this.references.samples.find((item) => item.id === this.form.sampleId); }
  onSampleChange(): void { this.changeDetector.detectChanges(); }
  formReady(): boolean {
    return !!this.form.name.trim() && !!this.form.facility.trim() && !!this.form.owner.trim() && this.form.sampleId > 0 && this.form.methodId > 0;
  }

  canSubmit(item: DomainRecord): boolean {
    return item.status === 'draft' && this.canWrite();
  }
  canSign(item: DomainRecord): boolean {
    if (!this.canReview() || item.status !== 'peer_review' || !item.basis) return false;
    if (this.isBlocked(item)) return false;
    return item.reviewRequestedBy !== this.auth.session()?.username;
  }
  signTitle(item: DomainRecord): string {
    if (!this.canReview()) return '仅复核员/管理员可签发';
    if (item.reviewRequestedBy === this.auth.session()?.username) return '提交人不能签发自己的结果';
    if (this.isBlocked(item)) return this.blockReasons(item).join('；');
    return '签发前再次核对样本与方法依据';
  }

  openCreate(): void { this.form = this.emptyForm(); this.form.code = `RR-${String(Date.now()).slice(-6)}`; this.showCreate = true; this.changeDetector.detectChanges(); }
  closeCreate(): void { this.showCreate = false; this.changeDetector.detectChanges(); }

  async createReview(): Promise<void> {
    if (!this.form.sampleId || !this.form.methodId || !this.form.name || !this.form.facility || !this.form.owner) {
      this.storeStateError('请完整填写名称、区域、责任人，并选择在检样本与现行方法');
      return;
    }
    try {
      await this.store.createReview({ ...this.form, code: this.form.code || `RR-${String(Date.now()).slice(-6)}` });
      this.showCreate = false;
    } catch { /* error is exposed in store state */ } finally {
      this.syncReferences();
      this.changeDetector.detectChanges();
    }
  }

  confirmSubmit(item: DomainRecord): void {
    this.pending = { item, target: 'peer_review', title: '提交复核', message: '提交时将重新读取在检样本和现行方法，并冻结样本批次、状态与方法版本，写入审核记录。' };
    this.changeDetector.detectChanges();
  }
  confirmSign(item: DomainRecord): void {
    this.pending = { item, target: 'signed', title: '签发结果复核', message: '签发前会再次核对：样本仍须在检，方法须为提交时的现行版本。核对不通过将拦截签发并留存审核记录。' };
    this.changeDetector.detectChanges();
  }
  confirmReject(item: DomainRecord): void {
    this.pending = { item, target: 'rejected', title: '驳回复核单', message: '驳回后提交人需修正并重新提交复核。' };
    this.changeDetector.detectChanges();
  }
  closePending(): void { this.pending = null; this.changeDetector.detectChanges(); }

  async runPending(): Promise<void> {
    if (!this.pending) return;
    const { item, target } = this.pending;
    try {
      await this.store.transition('reviews', item, target);
      this.pending = null;
    } catch { /* error is exposed in store state */ } finally {
      this.changeDetector.detectChanges();
    }
  }

  private storeStateError(message: string): void {
    // Surface validation problems through the same alert channel as API errors.
    this.store.patchError?.(message);
  }
}
