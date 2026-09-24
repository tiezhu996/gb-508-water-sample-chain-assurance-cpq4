
import { Injectable } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { EntityStore } from './factory';
import { request } from '../api/client';
import type { CreateReviewInput, DomainRecord } from '../types/domain';

export interface ReviewReferenceState {
  samples: DomainRecord[];
  methods: DomainRecord[];
  loading: boolean;
  error: string;
}

const emptyReferences: ReviewReferenceState = { samples: [], methods: [], loading: false, error: '' };

@Injectable({ providedIn: 'root' })
export class ResultReviewStore extends EntityStore {
  private readonly referenceSubject = new BehaviorSubject<ReviewReferenceState>(emptyReferences);
  readonly references$ = this.referenceSubject.asObservable();

  get referenceSnapshot(): ReviewReferenceState { return this.referenceSubject.value; }

  // New review forms must read the latest sample/method state: only 在检
  // samples and 现行 methods are offered as selectable basis.
  async loadReferences(): Promise<void> {
    this.patchReferences({ loading: true, error: '' });
    try {
      const [samples, methods] = await Promise.all([
        request<DomainRecord[]>('/samples?page=1&pageSize=100&status=testing'),
        request<DomainRecord[]>('/methods?page=1&pageSize=100&status=active'),
      ]);
      this.referenceSubject.next({
        samples: samples.data.filter((item) => item.status === 'testing'),
        methods: methods.data.filter((item) => item.status === 'active'),
        loading: false,
        error: '',
      });
    } catch (error) {
      this.patchReferences({ loading: false, error: error instanceof Error ? error.message : String(error) });
    }
  }

  async createReview(input: CreateReviewInput): Promise<void> {
    this.patch({ loading: true });
    try {
      await request<DomainRecord>('/reviews', { method: 'POST', body: JSON.stringify(input) });
      await Promise.all([this.load('reviews'), this.loadReferences()]);
    } catch (error) {
      this.patch({ loading: false, error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  private patchReferences(value: Partial<ReviewReferenceState>): void {
    this.referenceSubject.next({ ...this.referenceSubject.value, ...value });
  }
}
