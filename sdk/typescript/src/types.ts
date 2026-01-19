/**
 * Memory types supported by MAIA
 */
export type MemoryType = 'semantic' | 'episodic' | 'working';

/**
 * Memory source indicating how the memory was created
 */
export type MemorySource = 'user' | 'extracted' | 'inferred' | 'imported';

/**
 * A single memory unit stored in MAIA
 */
export interface Memory {
  id: string;
  namespace: string;
  content: string;
  type: MemoryType;
  embedding?: number[];
  metadata?: Record<string, unknown>;
  tags?: string[];
  created_at: string;
  updated_at: string;
  accessed_at: string;
  access_count: number;
  confidence: number;
  source: MemorySource;
}

/**
 * Configuration for a namespace
 */
export interface NamespaceConfig {
  token_budget?: number;
  max_memories?: number;
  retention_days?: number;
  allowed_types?: MemoryType[];
  inherit_from_parent?: boolean;
  custom_scoring?: Record<string, number>;
}

/**
 * A memory namespace
 */
export interface Namespace {
  id: string;
  name: string;
  parent?: string;
  template?: string;
  config: NamespaceConfig;
  created_at: string;
  updated_at: string;
}

/**
 * Input for creating a new memory
 */
export interface CreateMemoryInput {
  namespace: string;
  content: string;
  type?: MemoryType;
  metadata?: Record<string, unknown>;
  tags?: string[];
  confidence?: number;
  source?: MemorySource;
}

/**
 * Input for updating an existing memory
 */
export interface UpdateMemoryInput {
  content?: string;
  metadata?: Record<string, unknown>;
  tags?: string[];
  confidence?: number;
}

/**
 * Input for searching memories
 */
export interface SearchMemoriesInput {
  query?: string;
  namespace?: string;
  types?: MemoryType[];
  tags?: string[];
  limit?: number;
  offset?: number;
}

/**
 * A memory search result with score
 */
export interface SearchResult {
  memory: Memory;
  score: number;
}

/**
 * Input for creating a namespace
 */
export interface CreateNamespaceInput {
  name: string;
  parent?: string;
  template?: string;
  config?: NamespaceConfig;
}

/**
 * Input for updating a namespace
 */
export interface UpdateNamespaceInput {
  config: NamespaceConfig;
}

/**
 * Options for listing items
 */
export interface ListOptions {
  limit?: number;
  offset?: number;
}

/**
 * A paginated list response
 */
export interface ListResponse<T> {
  data: T[];
  count: number;
  offset: number;
  limit: number;
}

/**
 * Input for getting assembled context
 */
export interface GetContextInput {
  query: string;
  namespace?: string;
  token_budget?: number;
  system_prompt?: string;
  include_scores?: boolean;
  min_score?: number;
}

/**
 * A memory in the context response
 */
export interface ContextMemory {
  id: string;
  content: string;
  type: string;
  score?: number;
  position: string;
  token_count: number;
  truncated: boolean;
}

/**
 * Zone statistics in context response
 */
export interface ContextZoneStats {
  critical_used: number;
  critical_budget: number;
  middle_used: number;
  middle_budget: number;
  recency_used: number;
  recency_budget: number;
}

/**
 * The assembled context response
 */
export interface ContextResponse {
  content: string;
  memories: ContextMemory[];
  token_count: number;
  token_budget: number;
  truncated: boolean;
  zone_stats?: ContextZoneStats;
  query_time: string;
}

/**
 * Storage statistics
 */
export interface Stats {
  total_memories: number;
  total_namespaces: number;
  storage_size_bytes: number;
  last_compaction: string;
}

/**
 * Health check response
 */
export interface HealthResponse {
  status: string;
  service: string;
}

/**
 * API error response
 */
export interface APIErrorResponse {
  error: string;
  code?: string;
  details?: string;
}
