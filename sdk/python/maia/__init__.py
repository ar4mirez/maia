"""MAIA SDK - Python client for the MAIA memory system."""

from maia.client import MAIAClient, AsyncMAIAClient
from maia.types import (
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
)
from maia.errors import (
    MAIAError,
    APIError,
    ValidationError,
    NetworkError,
    NotFoundError,
    AlreadyExistsError,
)

__version__ = "0.1.0"

__all__ = [
    # Client
    "MAIAClient",
    "AsyncMAIAClient",
    # Types
    "Memory",
    "MemoryType",
    "MemorySource",
    "Namespace",
    "NamespaceConfig",
    "CreateMemoryInput",
    "UpdateMemoryInput",
    "SearchMemoriesInput",
    "SearchResult",
    "CreateNamespaceInput",
    "UpdateNamespaceInput",
    "ListOptions",
    "ListResponse",
    "GetContextInput",
    "ContextResponse",
    "ContextMemory",
    "ContextZoneStats",
    "Stats",
    "HealthResponse",
    # Errors
    "MAIAError",
    "APIError",
    "ValidationError",
    "NetworkError",
    "NotFoundError",
    "AlreadyExistsError",
]
