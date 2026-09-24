
export interface ReviewBasis {
  sampleId: number;
  sampleCode: string;
  sampleName: string;
  sampleBatch: string;
  sampleSnapshot: string;
  sampleStatus: string;
  sampleMissing: boolean;
  methodId: number;
  methodCode: string;
  methodName: string;
  methodSnapshot: number;
  methodVersion: number;
  methodStatus: string;
  methodMissing: boolean;
  eligible: boolean;
  reasons: string[];
  locked: boolean;
  lockedReason?: string;
}

export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  reviewRequestedBy?: string;
  peerReviewedBy?: string;
  signedBy?: string;
  // Frozen signing basis for 结果复核: sample batch/status and method version
  // captured when the review is submitted to peer review.
  sampleId?: number;
  sampleCode?: string;
  sampleBatch?: string;
  sampleSnapshot?: string;
  methodId?: number;
  methodCode?: string;
  methodSnapshot?: number;
  methodStatus?: string;
  basisBlockedAt?: string;
  basisBlockedReason?: string;
  basis?: ReviewBasis;
  createdAt: string;
  updatedAt: string;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }

export interface CreateReviewInput {
  code: string;
  name: string;
  description?: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: string;
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence?: string;
  sampleId: number;
  methodId: number;
}
