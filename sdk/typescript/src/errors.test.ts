import { describe, it, expect } from 'vitest';
import {
  MAIAError,
  APIError,
  ValidationError,
  NetworkError,
  isAPIError,
  isNotFoundError,
  isAlreadyExistsError,
} from './errors';

describe('MAIAError', () => {
  it('should create a MAIAError', () => {
    const error = new MAIAError('test error');
    expect(error.message).toBe('test error');
    expect(error.name).toBe('MAIAError');
    expect(error).toBeInstanceOf(Error);
  });
});

describe('APIError', () => {
  it('should create an APIError', () => {
    const error = new APIError(404, {
      error: 'not found',
      code: 'NOT_FOUND',
      details: 'memory not found',
    });

    expect(error.message).toBe('not found');
    expect(error.statusCode).toBe(404);
    expect(error.code).toBe('NOT_FOUND');
    expect(error.details).toBe('memory not found');
  });

  it('should detect not found errors', () => {
    const error = new APIError(404, { error: 'not found', code: 'NOT_FOUND' });
    expect(error.isNotFound()).toBe(true);
    expect(error.isAlreadyExists()).toBe(false);
    expect(error.isServerError()).toBe(false);
  });

  it('should detect already exists errors', () => {
    const error = new APIError(409, { error: 'exists', code: 'ALREADY_EXISTS' });
    expect(error.isAlreadyExists()).toBe(true);
    expect(error.isNotFound()).toBe(false);
  });

  it('should detect invalid input errors', () => {
    const error = new APIError(400, { error: 'invalid', code: 'INVALID_INPUT' });
    expect(error.isInvalidInput()).toBe(true);
  });

  it('should detect server errors', () => {
    const error = new APIError(500, { error: 'internal error' });
    expect(error.isServerError()).toBe(true);
  });
});

describe('ValidationError', () => {
  it('should create a ValidationError', () => {
    const error = new ValidationError('name', 'name is required');
    expect(error.message).toBe('name is required');
    expect(error.field).toBe('name');
    expect(error.name).toBe('ValidationError');
  });
});

describe('NetworkError', () => {
  it('should create a NetworkError', () => {
    const cause = new Error('connection refused');
    const error = new NetworkError('request failed', cause);
    expect(error.message).toBe('request failed');
    expect(error.cause).toBe(cause);
    expect(error.name).toBe('NetworkError');
  });

  it('should work without cause', () => {
    const error = new NetworkError('timeout');
    expect(error.cause).toBeUndefined();
  });
});

describe('isAPIError', () => {
  it('should return true for APIError', () => {
    const error = new APIError(404, { error: 'not found' });
    expect(isAPIError(error)).toBe(true);
  });

  it('should return false for other errors', () => {
    expect(isAPIError(new Error('test'))).toBe(false);
    expect(isAPIError(null)).toBe(false);
    expect(isAPIError(undefined)).toBe(false);
    expect(isAPIError('error')).toBe(false);
  });
});

describe('isNotFoundError', () => {
  it('should return true for not found APIError', () => {
    const error = new APIError(404, { error: 'not found' });
    expect(isNotFoundError(error)).toBe(true);
  });

  it('should return false for other errors', () => {
    expect(isNotFoundError(new APIError(500, { error: 'server' }))).toBe(false);
    expect(isNotFoundError(new Error('test'))).toBe(false);
    expect(isNotFoundError(null)).toBe(false);
  });
});

describe('isAlreadyExistsError', () => {
  it('should return true for already exists APIError', () => {
    const error = new APIError(409, { error: 'exists' });
    expect(isAlreadyExistsError(error)).toBe(true);
  });

  it('should return false for other errors', () => {
    expect(isAlreadyExistsError(new APIError(404, { error: 'not found' }))).toBe(
      false,
    );
    expect(isAlreadyExistsError(new Error('test'))).toBe(false);
    expect(isAlreadyExistsError(null)).toBe(false);
  });
});
