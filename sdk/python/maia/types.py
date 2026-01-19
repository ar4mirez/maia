"""Type definitions for the MAIA SDK."""

from datetime import datetime
from enum import Enum
from typing import Any, Generic, TypeVar

from pydantic import BaseModel, Field


class MemoryType(str, Enum):
    """Memory types supported by MAIA."""

    SEMANTIC = "semantic"
    EPISODIC = "episodic"
    WORKING = "working"


class MemorySource(str, Enum):
    """Memory source indicating how the memory was created."""

    USER = "user"
    EXTRACTED = "extracted"
    INFERRED = "inferred"
    IMPORTED = "imported"


class Memory(BaseModel):
    """A single memory unit stored in MAIA."""

    id: str
    namespace: str
    content: str
    type: MemoryType
    embedding: list[float] | None = None
    metadata: dict[str, Any] | None = None
    tags: list[str] | None = None
    created_at: datetime
    updated_at: datetime
    accessed_at: datetime
    access_count: int
    confidence: float
    source: MemorySource


class NamespaceConfig(BaseModel):
    """Configuration for a namespace."""

    token_budget: int | None = None
    max_memories: int | None = None
    retention_days: int | None = None
    allowed_types: list[MemoryType] | None = None
    inherit_from_parent: bool = False
    custom_scoring: dict[str, float] | None = None


class Namespace(BaseModel):
    """A memory namespace."""

    id: str
    name: str
    parent: str | None = None
    template: str | None = None
    config: NamespaceConfig
    created_at: datetime
    updated_at: datetime


class CreateMemoryInput(BaseModel):
    """Input for creating a new memory."""

    namespace: str
    content: str
    type: MemoryType | None = None
    metadata: dict[str, Any] | None = None
    tags: list[str] | None = None
    confidence: float | None = None
    source: MemorySource | None = None


class UpdateMemoryInput(BaseModel):
    """Input for updating an existing memory."""

    content: str | None = None
    metadata: dict[str, Any] | None = None
    tags: list[str] | None = None
    confidence: float | None = None


class SearchMemoriesInput(BaseModel):
    """Input for searching memories."""

    query: str | None = None
    namespace: str | None = None
    types: list[MemoryType] | None = None
    tags: list[str] | None = None
    limit: int | None = None
    offset: int | None = None


class SearchResult(BaseModel):
    """A memory search result with score."""

    memory: Memory
    score: float


class CreateNamespaceInput(BaseModel):
    """Input for creating a namespace."""

    name: str
    parent: str | None = None
    template: str | None = None
    config: NamespaceConfig | None = None


class UpdateNamespaceInput(BaseModel):
    """Input for updating a namespace."""

    config: NamespaceConfig


class ListOptions(BaseModel):
    """Options for listing items."""

    limit: int | None = None
    offset: int | None = None


T = TypeVar("T")


class ListResponse(BaseModel, Generic[T]):
    """A paginated list response."""

    data: list[T]
    count: int
    offset: int
    limit: int


class GetContextInput(BaseModel):
    """Input for getting assembled context."""

    query: str
    namespace: str | None = None
    token_budget: int | None = None
    system_prompt: str | None = None
    include_scores: bool | None = None
    min_score: float | None = None


class ContextMemory(BaseModel):
    """A memory in the context response."""

    id: str
    content: str
    type: str
    score: float | None = None
    position: str
    token_count: int
    truncated: bool


class ContextZoneStats(BaseModel):
    """Zone statistics in context response."""

    critical_used: int
    critical_budget: int
    middle_used: int
    middle_budget: int
    recency_used: int
    recency_budget: int


class ContextResponse(BaseModel):
    """The assembled context response."""

    content: str
    memories: list[ContextMemory]
    token_count: int
    token_budget: int
    truncated: bool
    zone_stats: ContextZoneStats | None = None
    query_time: str


class Stats(BaseModel):
    """Storage statistics."""

    total_memories: int
    total_namespaces: int
    storage_size_bytes: int
    last_compaction: datetime


class HealthResponse(BaseModel):
    """Health check response."""

    status: str
    service: str


class DeleteResponse(BaseModel):
    """Delete operation response."""

    deleted: bool


class APIErrorResponse(BaseModel):
    """API error response."""

    error: str
    code: str | None = None
    details: str | None = None
