// Main exports
export { MAIAClient, createClient } from './client';
export type { ClientOptions, RecallOptions } from './client';

// Type exports
export type {
  Memory,
  MemoryType,
  MemorySource,
  Namespace,
  NamespaceConfig,
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
  ContextMemory,
  ContextZoneStats,
  Stats,
  HealthResponse,
  APIErrorResponse,
} from './types';

// Error exports
export {
  MAIAError,
  APIError,
  ValidationError,
  NetworkError,
  isAPIError,
  isNotFoundError,
  isAlreadyExistsError,
} from './errors';
