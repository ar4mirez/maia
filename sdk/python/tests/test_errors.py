"""Tests for the MAIA SDK errors."""

import pytest

from maia.errors import (
    MAIAError,
    APIError,
    ValidationError,
    NetworkError,
    NotFoundError,
    AlreadyExistsError,
)


class TestMAIAError:
    """Tests for MAIAError."""

    def test_create_error(self) -> None:
        """Test creating a MAIAError."""
        error = MAIAError("test error")
        assert str(error) == "test error"
        assert isinstance(error, Exception)


class TestAPIError:
    """Tests for APIError."""

    def test_create_error(self) -> None:
        """Test creating an APIError."""
        error = APIError(
            status_code=404,
            message="not found",
            code="NOT_FOUND",
            details="memory not found",
        )
        assert error.status_code == 404
        assert error.message == "not found"
        assert error.code == "NOT_FOUND"
        assert error.details == "memory not found"

    def test_str_with_code(self) -> None:
        """Test string representation with code."""
        error = APIError(404, "not found", "NOT_FOUND")
        assert str(error) == "not found (NOT_FOUND)"

    def test_str_without_code(self) -> None:
        """Test string representation without code."""
        error = APIError(500, "internal error")
        assert str(error) == "internal error"

    def test_is_not_found(self) -> None:
        """Test is_not_found method."""
        error = APIError(404, "not found", "NOT_FOUND")
        assert error.is_not_found() is True
        assert error.is_already_exists() is False

    def test_is_already_exists(self) -> None:
        """Test is_already_exists method."""
        error = APIError(409, "exists", "ALREADY_EXISTS")
        assert error.is_already_exists() is True
        assert error.is_not_found() is False

    def test_is_invalid_input(self) -> None:
        """Test is_invalid_input method."""
        error = APIError(400, "invalid", "INVALID_INPUT")
        assert error.is_invalid_input() is True

    def test_is_server_error(self) -> None:
        """Test is_server_error method."""
        error = APIError(500, "internal error")
        assert error.is_server_error() is True

        error = APIError(404, "not found")
        assert error.is_server_error() is False


class TestValidationError:
    """Tests for ValidationError."""

    def test_create_error(self) -> None:
        """Test creating a ValidationError."""
        error = ValidationError("name", "name is required")
        assert error.field == "name"
        assert error.message == "name is required"
        assert str(error) == "name is required"


class TestNetworkError:
    """Tests for NetworkError."""

    def test_create_error(self) -> None:
        """Test creating a NetworkError."""
        cause = Exception("connection refused")
        error = NetworkError("request failed", cause)
        assert error.message == "request failed"
        assert error.__cause__ == cause

    def test_create_error_without_cause(self) -> None:
        """Test creating a NetworkError without cause."""
        error = NetworkError("timeout")
        assert error.__cause__ is None


class TestNotFoundError:
    """Tests for NotFoundError."""

    def test_create_error(self) -> None:
        """Test creating a NotFoundError."""
        error = NotFoundError("memory", "123")
        assert error.resource == "memory"
        assert error.id == "123"
        assert error.is_not_found() is True
        assert str(error) == "memory not found: 123 (NOT_FOUND)"


class TestAlreadyExistsError:
    """Tests for AlreadyExistsError."""

    def test_create_error(self) -> None:
        """Test creating an AlreadyExistsError."""
        error = AlreadyExistsError("namespace", "test")
        assert error.resource == "namespace"
        assert error.id == "test"
        assert error.is_already_exists() is True
