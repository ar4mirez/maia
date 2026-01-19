"""Error classes for the MAIA SDK."""

from typing import Any


class MAIAError(Exception):
    """Base error class for MAIA SDK errors."""

    pass


class APIError(MAIAError):
    """Error returned by the MAIA API."""

    def __init__(
        self,
        status_code: int,
        message: str,
        code: str | None = None,
        details: str | None = None,
    ) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.message = message
        self.code = code
        self.details = details

    def is_not_found(self) -> bool:
        """Returns True if the error is a not found error."""
        return self.status_code == 404 or self.code == "NOT_FOUND"

    def is_already_exists(self) -> bool:
        """Returns True if the error is an already exists error."""
        return self.status_code == 409 or self.code == "ALREADY_EXISTS"

    def is_invalid_input(self) -> bool:
        """Returns True if the error is an invalid input error."""
        return self.status_code == 400 or self.code == "INVALID_INPUT"

    def is_server_error(self) -> bool:
        """Returns True if the error is a server error."""
        return self.status_code >= 500

    def __str__(self) -> str:
        if self.code:
            return f"{self.message} ({self.code})"
        return self.message


class ValidationError(MAIAError):
    """Error thrown when a required field is missing."""

    def __init__(self, field: str, message: str) -> None:
        super().__init__(message)
        self.field = field
        self.message = message


class NetworkError(MAIAError):
    """Error thrown when a network request fails."""

    def __init__(self, message: str, cause: Exception | None = None) -> None:
        super().__init__(message)
        self.message = message
        self.__cause__ = cause


class NotFoundError(APIError):
    """Error thrown when a resource is not found."""

    def __init__(self, resource: str, id: str) -> None:
        super().__init__(404, f"{resource} not found: {id}", "NOT_FOUND")
        self.resource = resource
        self.id = id


class AlreadyExistsError(APIError):
    """Error thrown when a resource already exists."""

    def __init__(self, resource: str, id: str) -> None:
        super().__init__(409, f"{resource} already exists: {id}", "ALREADY_EXISTS")
        self.resource = resource
        self.id = id
