import {
  APIError,
  NetworkError,
  ValidationError,
} from './errors';
import type {
  Memory,
  Namespace,
  CreateMemoryInput,
  UpdateMemoryInput,
  SearchMemoriesInput,
  SearchResult,
  CreateNamespaceInput,
  UpdateNamespaceInput,
  ListOptions,
  ListResponse,
  GetContextInput,
  ContextResponse,
  Stats,
  HealthResponse,
  APIErrorResponse,
} from './types';

/**
 * Default configuration values
 */
const DEFAULT_BASE_URL = 'http://localhost:8080';
const DEFAULT_TIMEOUT = 30000;

/**
 * Client configuration options
 */
export interface ClientOptions {
  /** Base URL of the MAIA server */
  baseUrl?: string;
  /** Request timeout in milliseconds */
  timeout?: number;
  /** Custom headers to include in all requests */
  headers?: Record<string, string>;
  /** Custom fetch implementation */
  fetch?: typeof fetch;
}

/**
 * Options for the recall method
 */
export interface RecallOptions {
  /** Target namespace */
  namespace?: string;
  /** Token budget for context */
  tokenBudget?: number;
  /** System prompt to prepend */
  systemPrompt?: string;
  /** Minimum score threshold */
  minScore?: number;
  /** Include scores in response */
  includeScores?: boolean;
}

/**
 * MAIA SDK Client
 *
 * A TypeScript client for interacting with the MAIA memory system.
 *
 * @example
 * ```typescript
 * const client = new MAIAClient({ baseUrl: 'http://localhost:8080' });
 *
 * // Store a memory
 * const memory = await client.remember('default', 'User prefers dark mode');
 *
 * // Recall context
 * const context = await client.recall('What are the user preferences?');
 * ```
 */
export class MAIAClient {
  private readonly baseUrl: string;
  private readonly timeout: number;
  private readonly headers: Record<string, string>;
  private readonly fetchFn: typeof fetch;

  constructor(options: ClientOptions = {}) {
    this.baseUrl = options.baseUrl ?? DEFAULT_BASE_URL;
    this.timeout = options.timeout ?? DEFAULT_TIMEOUT;
    this.headers = options.headers ?? {};
    this.fetchFn = options.fetch ?? fetch;
  }

  /**
   * Perform an HTTP request
   */
  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await this.fetchFn(url, {
        method,
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
          ...this.headers,
        },
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      const text = await response.text();
      let data: unknown;

      if (text) {
        try {
          data = JSON.parse(text);
        } catch {
          data = text;
        }
      }

      if (!response.ok) {
        const errorResponse = data as APIErrorResponse;
        throw new APIError(response.status, {
          error: errorResponse?.error ?? `HTTP ${response.status}`,
          code: errorResponse?.code,
          details: errorResponse?.details,
        });
      }

      return data as T;
    } catch (error) {
      clearTimeout(timeoutId);

      if (error instanceof APIError) {
        throw error;
      }

      if (error instanceof Error) {
        if (error.name === 'AbortError') {
          throw new NetworkError('Request timeout');
        }
        throw new NetworkError(`Request failed: ${error.message}`, error);
      }

      throw new NetworkError('Request failed');
    }
  }

  // ============== Health ==============

  /**
   * Check if the server is healthy
   */
  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>('GET', '/health');
  }

  /**
   * Check if the server is ready to serve requests
   */
  async ready(): Promise<void> {
    await this.request<{ status: string }>('GET', '/ready');
  }

  /**
   * Get storage statistics
   */
  async stats(): Promise<Stats> {
    return this.request<Stats>('GET', '/v1/stats');
  }

  // ============== Memories ==============

  /**
   * Create a new memory
   */
  async createMemory(input: CreateMemoryInput): Promise<Memory> {
    if (!input.namespace) {
      throw new ValidationError('namespace', 'namespace is required');
    }
    if (!input.content) {
      throw new ValidationError('content', 'content is required');
    }

    return this.request<Memory>('POST', '/v1/memories', input);
  }

  /**
   * Get a memory by ID
   */
  async getMemory(id: string): Promise<Memory> {
    if (!id) {
      throw new ValidationError('id', 'id is required');
    }

    return this.request<Memory>('GET', `/v1/memories/${encodeURIComponent(id)}`);
  }

  /**
   * Update an existing memory
   */
  async updateMemory(id: string, input: UpdateMemoryInput): Promise<Memory> {
    if (!id) {
      throw new ValidationError('id', 'id is required');
    }

    return this.request<Memory>(
      'PUT',
      `/v1/memories/${encodeURIComponent(id)}`,
      input,
    );
  }

  /**
   * Delete a memory by ID
   */
  async deleteMemory(id: string): Promise<void> {
    if (!id) {
      throw new ValidationError('id', 'id is required');
    }

    await this.request<{ deleted: boolean }>(
      'DELETE',
      `/v1/memories/${encodeURIComponent(id)}`,
    );
  }

  /**
   * Search for memories
   */
  async searchMemories(
    input: SearchMemoriesInput = {},
  ): Promise<ListResponse<SearchResult>> {
    return this.request<ListResponse<SearchResult>>(
      'POST',
      '/v1/memories/search',
      input,
    );
  }

  // ============== Namespaces ==============

  /**
   * Create a new namespace
   */
  async createNamespace(input: CreateNamespaceInput): Promise<Namespace> {
    if (!input.name) {
      throw new ValidationError('name', 'name is required');
    }

    return this.request<Namespace>('POST', '/v1/namespaces', input);
  }

  /**
   * Get a namespace by ID or name
   */
  async getNamespace(idOrName: string): Promise<Namespace> {
    if (!idOrName) {
      throw new ValidationError('idOrName', 'id or name is required');
    }

    return this.request<Namespace>(
      'GET',
      `/v1/namespaces/${encodeURIComponent(idOrName)}`,
    );
  }

  /**
   * Update a namespace configuration
   */
  async updateNamespace(
    id: string,
    input: UpdateNamespaceInput,
  ): Promise<Namespace> {
    if (!id) {
      throw new ValidationError('id', 'id is required');
    }

    return this.request<Namespace>(
      'PUT',
      `/v1/namespaces/${encodeURIComponent(id)}`,
      input,
    );
  }

  /**
   * Delete a namespace
   */
  async deleteNamespace(id: string): Promise<void> {
    if (!id) {
      throw new ValidationError('id', 'id is required');
    }

    await this.request<{ deleted: boolean }>(
      'DELETE',
      `/v1/namespaces/${encodeURIComponent(id)}`,
    );
  }

  /**
   * List all namespaces
   */
  async listNamespaces(
    options: ListOptions = {},
  ): Promise<ListResponse<Namespace>> {
    const params = new URLSearchParams();
    if (options.limit) params.set('limit', options.limit.toString());
    if (options.offset) params.set('offset', options.offset.toString());

    const query = params.toString();
    const path = `/v1/namespaces${query ? `?${query}` : ''}`;

    return this.request<ListResponse<Namespace>>('GET', path);
  }

  /**
   * List memories in a namespace
   */
  async listNamespaceMemories(
    namespace: string,
    options: ListOptions = {},
  ): Promise<ListResponse<Memory>> {
    if (!namespace) {
      throw new ValidationError('namespace', 'namespace is required');
    }

    const params = new URLSearchParams();
    if (options.limit) params.set('limit', options.limit.toString());
    if (options.offset) params.set('offset', options.offset.toString());

    const query = params.toString();
    const path = `/v1/namespaces/${encodeURIComponent(namespace)}/memories${query ? `?${query}` : ''}`;

    return this.request<ListResponse<Memory>>('GET', path);
  }

  // ============== Context ==============

  /**
   * Get assembled context for a query
   */
  async getContext(input: GetContextInput): Promise<ContextResponse> {
    if (!input.query) {
      throw new ValidationError('query', 'query is required');
    }

    return this.request<ContextResponse>('POST', '/v1/context', input);
  }

  // ============== Convenience Methods ==============

  /**
   * Store a semantic memory (convenience method)
   *
   * @example
   * ```typescript
   * const memory = await client.remember('default', 'User prefers dark mode');
   * ```
   */
  async remember(namespace: string, content: string): Promise<Memory> {
    return this.createMemory({
      namespace,
      content,
      type: 'semantic',
      source: 'user',
      confidence: 1.0,
    });
  }

  /**
   * Recall context for a query (convenience method)
   *
   * @example
   * ```typescript
   * const context = await client.recall('What does the user prefer?', {
   *   namespace: 'default',
   *   tokenBudget: 2000,
   * });
   * ```
   */
  async recall(
    query: string,
    options: RecallOptions = {},
  ): Promise<ContextResponse> {
    return this.getContext({
      query,
      namespace: options.namespace,
      token_budget: options.tokenBudget,
      system_prompt: options.systemPrompt,
      min_score: options.minScore,
      include_scores: options.includeScores,
    });
  }

  /**
   * Delete a memory (convenience method)
   *
   * @example
   * ```typescript
   * await client.forget('mem-123');
   * ```
   */
  async forget(id: string): Promise<void> {
    return this.deleteMemory(id);
  }
}

/**
 * Create a new MAIA client
 *
 * @example
 * ```typescript
 * const client = createClient({ baseUrl: 'http://localhost:8080' });
 * ```
 */
export function createClient(options: ClientOptions = {}): MAIAClient {
  return new MAIAClient(options);
}
