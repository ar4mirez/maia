import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MAIAClient, createClient } from './client';
import { APIError, ValidationError, NetworkError } from './errors';

// Mock fetch
const mockFetch = vi.fn();

function createMockClient() {
  return new MAIAClient({
    baseUrl: 'http://localhost:8080',
    fetch: mockFetch as unknown as typeof fetch,
  });
}

function mockResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  });
}

describe('MAIAClient', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('constructor', () => {
    it('should use default options', () => {
      const client = new MAIAClient();
      expect(client).toBeInstanceOf(MAIAClient);
    });

    it('should accept custom options', () => {
      const client = new MAIAClient({
        baseUrl: 'http://custom:9000',
        timeout: 5000,
        headers: { 'X-Custom': 'value' },
      });
      expect(client).toBeInstanceOf(MAIAClient);
    });
  });

  describe('health', () => {
    it('should return health response', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({ status: 'healthy', service: 'maia' }),
      );

      const client = createMockClient();
      const result = await client.health();

      expect(result).toEqual({ status: 'healthy', service: 'maia' });
      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/health',
        expect.objectContaining({ method: 'GET' }),
      );
    });
  });

  describe('stats', () => {
    it('should return stats', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({ total_memories: 100, total_namespaces: 5 }),
      );

      const client = createMockClient();
      const result = await client.stats();

      expect(result.total_memories).toBe(100);
      expect(result.total_namespaces).toBe(5);
    });
  });

  describe('createMemory', () => {
    it('should create a memory', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'mem-123',
          namespace: 'test',
          content: 'Test memory',
          type: 'semantic',
        }),
      );

      const client = createMockClient();
      const result = await client.createMemory({
        namespace: 'test',
        content: 'Test memory',
        type: 'semantic',
      });

      expect(result.id).toBe('mem-123');
      expect(result.content).toBe('Test memory');
    });

    it('should throw ValidationError for missing namespace', async () => {
      const client = createMockClient();
      await expect(
        client.createMemory({ namespace: '', content: 'Test' }),
      ).rejects.toThrow(ValidationError);
    });

    it('should throw ValidationError for missing content', async () => {
      const client = createMockClient();
      await expect(
        client.createMemory({ namespace: 'test', content: '' }),
      ).rejects.toThrow(ValidationError);
    });

    it('should throw APIError on server error', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({ error: 'internal error', code: 'INTERNAL_ERROR' }, 500),
      );

      const client = createMockClient();
      await expect(
        client.createMemory({ namespace: 'test', content: 'Test' }),
      ).rejects.toThrow(APIError);
    });
  });

  describe('getMemory', () => {
    it('should get a memory by id', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'mem-123',
          content: 'Test memory',
        }),
      );

      const client = createMockClient();
      const result = await client.getMemory('mem-123');

      expect(result.id).toBe('mem-123');
      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/v1/memories/mem-123',
        expect.any(Object),
      );
    });

    it('should throw ValidationError for empty id', async () => {
      const client = createMockClient();
      await expect(client.getMemory('')).rejects.toThrow(ValidationError);
    });

    it('should throw APIError for not found', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({ error: 'memory not found', code: 'NOT_FOUND' }, 404),
      );

      const client = createMockClient();
      try {
        await client.getMemory('nonexistent');
        expect.fail('Should have thrown');
      } catch (error) {
        expect(error).toBeInstanceOf(APIError);
        expect((error as APIError).isNotFound()).toBe(true);
      }
    });
  });

  describe('updateMemory', () => {
    it('should update a memory', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'mem-123',
          content: 'Updated content',
        }),
      );

      const client = createMockClient();
      const result = await client.updateMemory('mem-123', {
        content: 'Updated content',
      });

      expect(result.content).toBe('Updated content');
    });

    it('should throw ValidationError for empty id', async () => {
      const client = createMockClient();
      await expect(
        client.updateMemory('', { content: 'test' }),
      ).rejects.toThrow(ValidationError);
    });
  });

  describe('deleteMemory', () => {
    it('should delete a memory', async () => {
      mockFetch.mockReturnValueOnce(mockResponse({ deleted: true }));

      const client = createMockClient();
      await expect(client.deleteMemory('mem-123')).resolves.not.toThrow();
    });

    it('should throw ValidationError for empty id', async () => {
      const client = createMockClient();
      await expect(client.deleteMemory('')).rejects.toThrow(ValidationError);
    });
  });

  describe('searchMemories', () => {
    it('should search memories', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          data: [
            { memory: { id: 'mem-1', content: 'Result 1' }, score: 0.9 },
            { memory: { id: 'mem-2', content: 'Result 2' }, score: 0.8 },
          ],
          count: 2,
          limit: 100,
          offset: 0,
        }),
      );

      const client = createMockClient();
      const result = await client.searchMemories({
        query: 'test',
        namespace: 'default',
      });

      expect(result.data).toHaveLength(2);
      expect(result.data[0]?.memory.id).toBe('mem-1');
    });
  });

  describe('createNamespace', () => {
    it('should create a namespace', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'ns-123',
          name: 'test-namespace',
        }),
      );

      const client = createMockClient();
      const result = await client.createNamespace({
        name: 'test-namespace',
        config: { token_budget: 4000 },
      });

      expect(result.id).toBe('ns-123');
      expect(result.name).toBe('test-namespace');
    });

    it('should throw ValidationError for missing name', async () => {
      const client = createMockClient();
      await expect(client.createNamespace({ name: '' })).rejects.toThrow(
        ValidationError,
      );
    });
  });

  describe('getNamespace', () => {
    it('should get a namespace by name', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'ns-123',
          name: 'test-namespace',
        }),
      );

      const client = createMockClient();
      const result = await client.getNamespace('test-namespace');

      expect(result.id).toBe('ns-123');
    });

    it('should throw ValidationError for empty id', async () => {
      const client = createMockClient();
      await expect(client.getNamespace('')).rejects.toThrow(ValidationError);
    });
  });

  describe('listNamespaces', () => {
    it('should list namespaces', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          data: [
            { id: 'ns-1', name: 'namespace-1' },
            { id: 'ns-2', name: 'namespace-2' },
          ],
          count: 2,
          limit: 100,
          offset: 0,
        }),
      );

      const client = createMockClient();
      const result = await client.listNamespaces({ limit: 10 });

      expect(result.data).toHaveLength(2);
    });

    it('should append query params', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({ data: [], count: 0, limit: 10, offset: 5 }),
      );

      const client = createMockClient();
      await client.listNamespaces({ limit: 10, offset: 5 });

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('limit=10'),
        expect.any(Object),
      );
    });
  });

  describe('listNamespaceMemories', () => {
    it('should list namespace memories', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          data: [
            { id: 'mem-1', content: 'Memory 1' },
            { id: 'mem-2', content: 'Memory 2' },
          ],
          count: 2,
          limit: 100,
          offset: 0,
        }),
      );

      const client = createMockClient();
      const result = await client.listNamespaceMemories('test');

      expect(result.data).toHaveLength(2);
    });

    it('should throw ValidationError for empty namespace', async () => {
      const client = createMockClient();
      await expect(client.listNamespaceMemories('')).rejects.toThrow(
        ValidationError,
      );
    });
  });

  describe('getContext', () => {
    it('should get context', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          content: 'User prefers dark mode.',
          token_count: 10,
          token_budget: 2000,
          truncated: false,
        }),
      );

      const client = createMockClient();
      const result = await client.getContext({
        query: 'What are the user preferences?',
        namespace: 'default',
        token_budget: 2000,
      });

      expect(result.content).toBe('User prefers dark mode.');
      expect(result.token_count).toBe(10);
    });

    it('should throw ValidationError for missing query', async () => {
      const client = createMockClient();
      await expect(
        client.getContext({ query: '', namespace: 'default' }),
      ).rejects.toThrow(ValidationError);
    });
  });

  describe('remember', () => {
    it('should create a semantic memory', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          id: 'mem-123',
          namespace: 'test',
          content: 'User likes coffee',
          type: 'semantic',
          source: 'user',
        }),
      );

      const client = createMockClient();
      const result = await client.remember('test', 'User likes coffee');

      expect(result.id).toBe('mem-123');
      expect(result.content).toBe('User likes coffee');
    });
  });

  describe('recall', () => {
    it('should recall context with options', async () => {
      mockFetch.mockReturnValueOnce(
        mockResponse({
          content: 'User likes coffee',
          token_count: 5,
        }),
      );

      const client = createMockClient();
      const result = await client.recall('user preferences', {
        namespace: 'test',
        tokenBudget: 1000,
        includeScores: true,
      });

      expect(result.content).toBe('User likes coffee');
    });
  });

  describe('forget', () => {
    it('should delete a memory', async () => {
      mockFetch.mockReturnValueOnce(mockResponse({ deleted: true }));

      const client = createMockClient();
      await expect(client.forget('mem-123')).resolves.not.toThrow();
    });
  });

  describe('error handling', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'));

      const client = createMockClient();
      await expect(client.health()).rejects.toThrow(NetworkError);
    });

    it('should handle timeout errors', async () => {
      const abortError = new Error('Aborted');
      abortError.name = 'AbortError';
      mockFetch.mockRejectedValueOnce(abortError);

      const client = createMockClient();
      await expect(client.health()).rejects.toThrow(NetworkError);
    });

    it('should handle non-JSON error responses', async () => {
      mockFetch.mockReturnValueOnce(
        Promise.resolve({
          ok: false,
          status: 500,
          text: () => Promise.resolve('Internal Server Error'),
        }),
      );

      const client = createMockClient();
      try {
        await client.health();
        expect.fail('Should have thrown');
      } catch (error) {
        expect(error).toBeInstanceOf(APIError);
        expect((error as APIError).statusCode).toBe(500);
      }
    });
  });
});

describe('createClient', () => {
  it('should create a new client', () => {
    const client = createClient({ baseUrl: 'http://test:8080' });
    expect(client).toBeInstanceOf(MAIAClient);
  });
});
