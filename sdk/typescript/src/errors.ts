import type { APIErrorResponse } from './types';

/**
 * Base error class for MAIA SDK errors
 */
export class MAIAError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'MAIAError';
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Error returned by the MAIA API
 */
export class APIError extends MAIAError {
  readonly statusCode: number;
  readonly code?: string;
  readonly details?: string;

  constructor(statusCode: number, response: APIErrorResponse) {
    super(response.error);
    this.name = 'APIError';
    this.statusCode = statusCode;
    this.code = response.code;
    this.details = response.details;
  }

  /**
   * Returns true if the error is a not found error
   */
  isNotFound(): boolean {
    return this.statusCode === 404 || this.code === 'NOT_FOUND';
  }

  /**
   * Returns true if the error is an already exists error
   */
  isAlreadyExists(): boolean {
    return this.statusCode === 409 || this.code === 'ALREADY_EXISTS';
  }

  /**
   * Returns true if the error is an invalid input error
   */
  isInvalidInput(): boolean {
    return this.statusCode === 400 || this.code === 'INVALID_INPUT';
  }

  /**
   * Returns true if the error is a server error
   */
  isServerError(): boolean {
    return this.statusCode >= 500;
  }
}

/**
 * Error thrown when a required field is missing
 */
export class ValidationError extends MAIAError {
  readonly field: string;

  constructor(field: string, message: string) {
    super(message);
    this.name = 'ValidationError';
    this.field = field;
  }
}

/**
 * Error thrown when a network request fails
 */
export class NetworkError extends MAIAError {
  readonly cause?: Error;

  constructor(message: string, cause?: Error) {
    super(message);
    this.name = 'NetworkError';
    this.cause = cause;
  }
}

/**
 * Type guard for APIError
 */
export function isAPIError(error: unknown): error is APIError {
  return error instanceof APIError;
}

/**
 * Type guard for not found errors
 */
export function isNotFoundError(error: unknown): boolean {
  return isAPIError(error) && error.isNotFound();
}

/**
 * Type guard for already exists errors
 */
export function isAlreadyExistsError(error: unknown): boolean {
  return isAPIError(error) && error.isAlreadyExists();
}
