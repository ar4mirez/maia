"""MAIA SDK Client."""

from typing import Any
from urllib.parse import quote, urlencode

import httpx

from maia.errors import APIError, NetworkError, ValidationError
from maia.types import (
    ContextResponse,
    CreateMemoryInput,
    CreateNamespaceInput,
    DeleteResponse,
    GetContextInput,
    HealthResponse,
    ListOptions,
    ListResponse,
    Memory,
    MemorySource,
    MemoryType,
    Namespace,
    SearchMemoriesInput,
    SearchResult,
    Stats,
    UpdateMemoryInput,
    UpdateNamespaceInput,
)

DEFAULT_BASE_URL = "http://localhost:8080"
DEFAULT_TIMEOUT = 30.0


class MAIAClient:
    """Synchronous MAIA SDK client.

    A Python client for interacting with the MAIA memory system.

    Example:
        ```python
        client = MAIAClient(base_url="http://localhost:8080")

        # Store a memory
        memory = client.remember("default", "User prefers dark mode")

        # Recall context
        context = client.recall("What are the user preferences?")
        ```
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        timeout: float = DEFAULT_TIMEOUT,
        headers: dict[str, str] | None = None,
    ) -> None:
        """Initialize the MAIA client.

        Args:
            base_url: Base URL of the MAIA server.
            timeout: Request timeout in seconds.
            headers: Custom headers to include in all requests.
        """
        self.base_url = base_url.rstrip("/")
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
            headers=headers or {},
        )

    def __enter__(self) -> "MAIAClient":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    def close(self) -> None:
        """Close the client."""
        self._client.close()

    def _request(
        self,
        method: str,
        path: str,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
    ) -> Any:
        """Perform an HTTP request."""
        try:
            response = self._client.request(
                method,
                path,
                json=json,
                params=params,
            )
        except httpx.RequestError as e:
            raise NetworkError(f"Request failed: {e}", e) from e

        if response.status_code >= 400:
            try:
                data = response.json()
                raise APIError(
                    status_code=response.status_code,
                    message=data.get("error", f"HTTP {response.status_code}"),
                    code=data.get("code"),
                    details=data.get("details"),
                )
            except ValueError:
                raise APIError(
                    status_code=response.status_code,
                    message=response.text or f"HTTP {response.status_code}",
                )

        if response.content:
            return response.json()
        return None

    # ============== Health ==============

    def health(self) -> HealthResponse:
        """Check if the server is healthy."""
        data = self._request("GET", "/health")
        return HealthResponse.model_validate(data)

    def ready(self) -> None:
        """Check if the server is ready to serve requests."""
        self._request("GET", "/ready")

    def stats(self) -> Stats:
        """Get storage statistics."""
        data = self._request("GET", "/v1/stats")
        return Stats.model_validate(data)

    # ============== Memories ==============

    def create_memory(self, input: CreateMemoryInput) -> Memory:
        """Create a new memory."""
        if not input.namespace:
            raise ValidationError("namespace", "namespace is required")
        if not input.content:
            raise ValidationError("content", "content is required")

        data = self._request(
            "POST",
            "/v1/memories",
            json=input.model_dump(exclude_none=True),
        )
        return Memory.model_validate(data)

    def get_memory(self, id: str) -> Memory:
        """Get a memory by ID."""
        if not id:
            raise ValidationError("id", "id is required")

        data = self._request("GET", f"/v1/memories/{quote(id, safe='')}")
        return Memory.model_validate(data)

    def update_memory(self, id: str, input: UpdateMemoryInput) -> Memory:
        """Update an existing memory."""
        if not id:
            raise ValidationError("id", "id is required")

        data = self._request(
            "PUT",
            f"/v1/memories/{quote(id, safe='')}",
            json=input.model_dump(exclude_none=True),
        )
        return Memory.model_validate(data)

    def delete_memory(self, id: str) -> None:
        """Delete a memory by ID."""
        if not id:
            raise ValidationError("id", "id is required")

        self._request("DELETE", f"/v1/memories/{quote(id, safe='')}")

    def search_memories(
        self, input: SearchMemoriesInput | None = None
    ) -> ListResponse[SearchResult]:
        """Search for memories."""
        if input is None:
            input = SearchMemoriesInput()

        data = self._request(
            "POST",
            "/v1/memories/search",
            json=input.model_dump(exclude_none=True),
        )
        return ListResponse[SearchResult].model_validate(data)

    # ============== Namespaces ==============

    def create_namespace(self, input: CreateNamespaceInput) -> Namespace:
        """Create a new namespace."""
        if not input.name:
            raise ValidationError("name", "name is required")

        data = self._request(
            "POST",
            "/v1/namespaces",
            json=input.model_dump(exclude_none=True),
        )
        return Namespace.model_validate(data)

    def get_namespace(self, id_or_name: str) -> Namespace:
        """Get a namespace by ID or name."""
        if not id_or_name:
            raise ValidationError("id_or_name", "id or name is required")

        data = self._request("GET", f"/v1/namespaces/{quote(id_or_name, safe='')}")
        return Namespace.model_validate(data)

    def update_namespace(self, id: str, input: UpdateNamespaceInput) -> Namespace:
        """Update a namespace configuration."""
        if not id:
            raise ValidationError("id", "id is required")

        data = self._request(
            "PUT",
            f"/v1/namespaces/{quote(id, safe='')}",
            json=input.model_dump(exclude_none=True),
        )
        return Namespace.model_validate(data)

    def delete_namespace(self, id: str) -> None:
        """Delete a namespace."""
        if not id:
            raise ValidationError("id", "id is required")

        self._request("DELETE", f"/v1/namespaces/{quote(id, safe='')}")

    def list_namespaces(
        self, options: ListOptions | None = None
    ) -> ListResponse[Namespace]:
        """List all namespaces."""
        params = {}
        if options:
            if options.limit:
                params["limit"] = options.limit
            if options.offset:
                params["offset"] = options.offset

        data = self._request("GET", "/v1/namespaces", params=params or None)
        return ListResponse[Namespace].model_validate(data)

    def list_namespace_memories(
        self, namespace: str, options: ListOptions | None = None
    ) -> ListResponse[Memory]:
        """List memories in a namespace."""
        if not namespace:
            raise ValidationError("namespace", "namespace is required")

        params = {}
        if options:
            if options.limit:
                params["limit"] = options.limit
            if options.offset:
                params["offset"] = options.offset

        data = self._request(
            "GET",
            f"/v1/namespaces/{quote(namespace, safe='')}/memories",
            params=params or None,
        )
        return ListResponse[Memory].model_validate(data)

    # ============== Context ==============

    def get_context(self, input: GetContextInput) -> ContextResponse:
        """Get assembled context for a query."""
        if not input.query:
            raise ValidationError("query", "query is required")

        data = self._request(
            "POST",
            "/v1/context",
            json=input.model_dump(exclude_none=True),
        )
        return ContextResponse.model_validate(data)

    # ============== Convenience Methods ==============

    def remember(self, namespace: str, content: str) -> Memory:
        """Store a semantic memory (convenience method).

        Example:
            ```python
            memory = client.remember("default", "User prefers dark mode")
            ```
        """
        return self.create_memory(
            CreateMemoryInput(
                namespace=namespace,
                content=content,
                type=MemoryType.SEMANTIC,
                source=MemorySource.USER,
                confidence=1.0,
            )
        )

    def recall(
        self,
        query: str,
        *,
        namespace: str | None = None,
        token_budget: int | None = None,
        system_prompt: str | None = None,
        min_score: float | None = None,
        include_scores: bool | None = None,
    ) -> ContextResponse:
        """Recall context for a query (convenience method).

        Example:
            ```python
            context = client.recall(
                "What does the user prefer?",
                namespace="default",
                token_budget=2000,
            )
            ```
        """
        return self.get_context(
            GetContextInput(
                query=query,
                namespace=namespace,
                token_budget=token_budget,
                system_prompt=system_prompt,
                min_score=min_score,
                include_scores=include_scores,
            )
        )

    def forget(self, id: str) -> None:
        """Delete a memory (convenience method).

        Example:
            ```python
            client.forget("mem-123")
            ```
        """
        self.delete_memory(id)


class AsyncMAIAClient:
    """Asynchronous MAIA SDK client.

    An async Python client for interacting with the MAIA memory system.

    Example:
        ```python
        async with AsyncMAIAClient(base_url="http://localhost:8080") as client:
            # Store a memory
            memory = await client.remember("default", "User prefers dark mode")

            # Recall context
            context = await client.recall("What are the user preferences?")
        ```
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        timeout: float = DEFAULT_TIMEOUT,
        headers: dict[str, str] | None = None,
    ) -> None:
        """Initialize the async MAIA client.

        Args:
            base_url: Base URL of the MAIA server.
            timeout: Request timeout in seconds.
            headers: Custom headers to include in all requests.
        """
        self.base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=timeout,
            headers=headers or {},
        )

    async def __aenter__(self) -> "AsyncMAIAClient":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    async def close(self) -> None:
        """Close the client."""
        await self._client.aclose()

    async def _request(
        self,
        method: str,
        path: str,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
    ) -> Any:
        """Perform an HTTP request."""
        try:
            response = await self._client.request(
                method,
                path,
                json=json,
                params=params,
            )
        except httpx.RequestError as e:
            raise NetworkError(f"Request failed: {e}", e) from e

        if response.status_code >= 400:
            try:
                data = response.json()
                raise APIError(
                    status_code=response.status_code,
                    message=data.get("error", f"HTTP {response.status_code}"),
                    code=data.get("code"),
                    details=data.get("details"),
                )
            except ValueError:
                raise APIError(
                    status_code=response.status_code,
                    message=response.text or f"HTTP {response.status_code}",
                )

        if response.content:
            return response.json()
        return None

    # ============== Health ==============

    async def health(self) -> HealthResponse:
        """Check if the server is healthy."""
        data = await self._request("GET", "/health")
        return HealthResponse.model_validate(data)

    async def ready(self) -> None:
        """Check if the server is ready to serve requests."""
        await self._request("GET", "/ready")

    async def stats(self) -> Stats:
        """Get storage statistics."""
        data = await self._request("GET", "/v1/stats")
        return Stats.model_validate(data)

    # ============== Memories ==============

    async def create_memory(self, input: CreateMemoryInput) -> Memory:
        """Create a new memory."""
        if not input.namespace:
            raise ValidationError("namespace", "namespace is required")
        if not input.content:
            raise ValidationError("content", "content is required")

        data = await self._request(
            "POST",
            "/v1/memories",
            json=input.model_dump(exclude_none=True),
        )
        return Memory.model_validate(data)

    async def get_memory(self, id: str) -> Memory:
        """Get a memory by ID."""
        if not id:
            raise ValidationError("id", "id is required")

        data = await self._request("GET", f"/v1/memories/{quote(id, safe='')}")
        return Memory.model_validate(data)

    async def update_memory(self, id: str, input: UpdateMemoryInput) -> Memory:
        """Update an existing memory."""
        if not id:
            raise ValidationError("id", "id is required")

        data = await self._request(
            "PUT",
            f"/v1/memories/{quote(id, safe='')}",
            json=input.model_dump(exclude_none=True),
        )
        return Memory.model_validate(data)

    async def delete_memory(self, id: str) -> None:
        """Delete a memory by ID."""
        if not id:
            raise ValidationError("id", "id is required")

        await self._request("DELETE", f"/v1/memories/{quote(id, safe='')}")

    async def search_memories(
        self, input: SearchMemoriesInput | None = None
    ) -> ListResponse[SearchResult]:
        """Search for memories."""
        if input is None:
            input = SearchMemoriesInput()

        data = await self._request(
            "POST",
            "/v1/memories/search",
            json=input.model_dump(exclude_none=True),
        )
        return ListResponse[SearchResult].model_validate(data)

    # ============== Namespaces ==============

    async def create_namespace(self, input: CreateNamespaceInput) -> Namespace:
        """Create a new namespace."""
        if not input.name:
            raise ValidationError("name", "name is required")

        data = await self._request(
            "POST",
            "/v1/namespaces",
            json=input.model_dump(exclude_none=True),
        )
        return Namespace.model_validate(data)

    async def get_namespace(self, id_or_name: str) -> Namespace:
        """Get a namespace by ID or name."""
        if not id_or_name:
            raise ValidationError("id_or_name", "id or name is required")

        data = await self._request(
            "GET", f"/v1/namespaces/{quote(id_or_name, safe='')}"
        )
        return Namespace.model_validate(data)

    async def update_namespace(
        self, id: str, input: UpdateNamespaceInput
    ) -> Namespace:
        """Update a namespace configuration."""
        if not id:
            raise ValidationError("id", "id is required")

        data = await self._request(
            "PUT",
            f"/v1/namespaces/{quote(id, safe='')}",
            json=input.model_dump(exclude_none=True),
        )
        return Namespace.model_validate(data)

    async def delete_namespace(self, id: str) -> None:
        """Delete a namespace."""
        if not id:
            raise ValidationError("id", "id is required")

        await self._request("DELETE", f"/v1/namespaces/{quote(id, safe='')}")

    async def list_namespaces(
        self, options: ListOptions | None = None
    ) -> ListResponse[Namespace]:
        """List all namespaces."""
        params = {}
        if options:
            if options.limit:
                params["limit"] = options.limit
            if options.offset:
                params["offset"] = options.offset

        data = await self._request("GET", "/v1/namespaces", params=params or None)
        return ListResponse[Namespace].model_validate(data)

    async def list_namespace_memories(
        self, namespace: str, options: ListOptions | None = None
    ) -> ListResponse[Memory]:
        """List memories in a namespace."""
        if not namespace:
            raise ValidationError("namespace", "namespace is required")

        params = {}
        if options:
            if options.limit:
                params["limit"] = options.limit
            if options.offset:
                params["offset"] = options.offset

        data = await self._request(
            "GET",
            f"/v1/namespaces/{quote(namespace, safe='')}/memories",
            params=params or None,
        )
        return ListResponse[Memory].model_validate(data)

    # ============== Context ==============

    async def get_context(self, input: GetContextInput) -> ContextResponse:
        """Get assembled context for a query."""
        if not input.query:
            raise ValidationError("query", "query is required")

        data = await self._request(
            "POST",
            "/v1/context",
            json=input.model_dump(exclude_none=True),
        )
        return ContextResponse.model_validate(data)

    # ============== Convenience Methods ==============

    async def remember(self, namespace: str, content: str) -> Memory:
        """Store a semantic memory (convenience method).

        Example:
            ```python
            memory = await client.remember("default", "User prefers dark mode")
            ```
        """
        return await self.create_memory(
            CreateMemoryInput(
                namespace=namespace,
                content=content,
                type=MemoryType.SEMANTIC,
                source=MemorySource.USER,
                confidence=1.0,
            )
        )

    async def recall(
        self,
        query: str,
        *,
        namespace: str | None = None,
        token_budget: int | None = None,
        system_prompt: str | None = None,
        min_score: float | None = None,
        include_scores: bool | None = None,
    ) -> ContextResponse:
        """Recall context for a query (convenience method).

        Example:
            ```python
            context = await client.recall(
                "What does the user prefer?",
                namespace="default",
                token_budget=2000,
            )
            ```
        """
        return await self.get_context(
            GetContextInput(
                query=query,
                namespace=namespace,
                token_budget=token_budget,
                system_prompt=system_prompt,
                min_score=min_score,
                include_scores=include_scores,
            )
        )

    async def forget(self, id: str) -> None:
        """Delete a memory (convenience method).

        Example:
            ```python
            await client.forget("mem-123")
            ```
        """
        await self.delete_memory(id)
