"""Tests for the MAIA SDK client."""

import pytest
import respx
from httpx import Response

from maia import (
    MAIAClient,
    AsyncMAIAClient,
    CreateMemoryInput,
    UpdateMemoryInput,
    SearchMemoriesInput,
    CreateNamespaceInput,
    UpdateNamespaceInput,
    GetContextInput,
    ListOptions,
    NamespaceConfig,
    MemoryType,
    MemorySource,
    APIError,
    ValidationError,
)


BASE_URL = "http://localhost:8080"


class TestMAIAClient:
    """Tests for the synchronous MAIA client."""

    @respx.mock
    def test_health(self) -> None:
        """Test health check."""
        respx.get(f"{BASE_URL}/health").mock(
            return_value=Response(200, json={"status": "healthy", "service": "maia"})
        )

        client = MAIAClient(base_url=BASE_URL)
        response = client.health()

        assert response.status == "healthy"
        assert response.service == "maia"

    @respx.mock
    def test_stats(self) -> None:
        """Test stats."""
        respx.get(f"{BASE_URL}/v1/stats").mock(
            return_value=Response(
                200,
                json={
                    "total_memories": 100,
                    "total_namespaces": 5,
                    "storage_size_bytes": 1024,
                    "last_compaction": "2024-01-01T00:00:00Z",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        response = client.stats()

        assert response.total_memories == 100
        assert response.total_namespaces == 5

    @respx.mock
    def test_create_memory(self) -> None:
        """Test creating a memory."""
        respx.post(f"{BASE_URL}/v1/memories").mock(
            return_value=Response(
                201,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "Test memory",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 0,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        memory = client.create_memory(
            CreateMemoryInput(
                namespace="test",
                content="Test memory",
                type=MemoryType.SEMANTIC,
            )
        )

        assert memory.id == "mem-123"
        assert memory.content == "Test memory"

    def test_create_memory_validation_error_namespace(self) -> None:
        """Test validation error for missing namespace."""
        client = MAIAClient(base_url=BASE_URL)
        with pytest.raises(ValidationError) as exc_info:
            client.create_memory(CreateMemoryInput(namespace="", content="Test"))
        assert exc_info.value.field == "namespace"

    def test_create_memory_validation_error_content(self) -> None:
        """Test validation error for missing content."""
        client = MAIAClient(base_url=BASE_URL)
        with pytest.raises(ValidationError) as exc_info:
            client.create_memory(CreateMemoryInput(namespace="test", content=""))
        assert exc_info.value.field == "content"

    @respx.mock
    def test_get_memory(self) -> None:
        """Test getting a memory."""
        respx.get(f"{BASE_URL}/v1/memories/mem-123").mock(
            return_value=Response(
                200,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "Test memory",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 1,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        memory = client.get_memory("mem-123")

        assert memory.id == "mem-123"

    def test_get_memory_validation_error(self) -> None:
        """Test validation error for missing id."""
        client = MAIAClient(base_url=BASE_URL)
        with pytest.raises(ValidationError) as exc_info:
            client.get_memory("")
        assert exc_info.value.field == "id"

    @respx.mock
    def test_get_memory_not_found(self) -> None:
        """Test getting a non-existent memory."""
        respx.get(f"{BASE_URL}/v1/memories/nonexistent").mock(
            return_value=Response(
                404, json={"error": "memory not found", "code": "NOT_FOUND"}
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        with pytest.raises(APIError) as exc_info:
            client.get_memory("nonexistent")

        assert exc_info.value.is_not_found()

    @respx.mock
    def test_update_memory(self) -> None:
        """Test updating a memory."""
        respx.put(f"{BASE_URL}/v1/memories/mem-123").mock(
            return_value=Response(
                200,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "Updated content",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 1,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        memory = client.update_memory(
            "mem-123", UpdateMemoryInput(content="Updated content")
        )

        assert memory.content == "Updated content"

    @respx.mock
    def test_delete_memory(self) -> None:
        """Test deleting a memory."""
        respx.delete(f"{BASE_URL}/v1/memories/mem-123").mock(
            return_value=Response(200, json={"deleted": True})
        )

        client = MAIAClient(base_url=BASE_URL)
        client.delete_memory("mem-123")  # Should not raise

    @respx.mock
    def test_search_memories(self) -> None:
        """Test searching memories."""
        respx.post(f"{BASE_URL}/v1/memories/search").mock(
            return_value=Response(
                200,
                json={
                    "data": [
                        {
                            "memory": {
                                "id": "mem-1",
                                "namespace": "test",
                                "content": "Result 1",
                                "type": "semantic",
                                "created_at": "2024-01-01T00:00:00Z",
                                "updated_at": "2024-01-01T00:00:00Z",
                                "accessed_at": "2024-01-01T00:00:00Z",
                                "access_count": 0,
                                "confidence": 1.0,
                                "source": "user",
                            },
                            "score": 0.9,
                        }
                    ],
                    "count": 1,
                    "offset": 0,
                    "limit": 100,
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        results = client.search_memories(
            SearchMemoriesInput(query="test", namespace="test")
        )

        assert len(results.data) == 1
        assert results.data[0].memory.id == "mem-1"

    @respx.mock
    def test_create_namespace(self) -> None:
        """Test creating a namespace."""
        respx.post(f"{BASE_URL}/v1/namespaces").mock(
            return_value=Response(
                201,
                json={
                    "id": "ns-123",
                    "name": "test-namespace",
                    "config": {"token_budget": 4000},
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        ns = client.create_namespace(
            CreateNamespaceInput(
                name="test-namespace", config=NamespaceConfig(token_budget=4000)
            )
        )

        assert ns.id == "ns-123"
        assert ns.name == "test-namespace"

    @respx.mock
    def test_get_namespace(self) -> None:
        """Test getting a namespace."""
        respx.get(f"{BASE_URL}/v1/namespaces/test-namespace").mock(
            return_value=Response(
                200,
                json={
                    "id": "ns-123",
                    "name": "test-namespace",
                    "config": {},
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        ns = client.get_namespace("test-namespace")

        assert ns.id == "ns-123"

    @respx.mock
    def test_list_namespaces(self) -> None:
        """Test listing namespaces."""
        respx.get(f"{BASE_URL}/v1/namespaces").mock(
            return_value=Response(
                200,
                json={
                    "data": [
                        {
                            "id": "ns-1",
                            "name": "namespace-1",
                            "config": {},
                            "created_at": "2024-01-01T00:00:00Z",
                            "updated_at": "2024-01-01T00:00:00Z",
                        }
                    ],
                    "count": 1,
                    "offset": 0,
                    "limit": 100,
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        results = client.list_namespaces(ListOptions(limit=10))

        assert len(results.data) == 1

    @respx.mock
    def test_get_context(self) -> None:
        """Test getting context."""
        respx.post(f"{BASE_URL}/v1/context").mock(
            return_value=Response(
                200,
                json={
                    "content": "User prefers dark mode.",
                    "memories": [],
                    "token_count": 10,
                    "token_budget": 2000,
                    "truncated": False,
                    "query_time": "1ms",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        response = client.get_context(
            GetContextInput(query="What are the user preferences?", namespace="default")
        )

        assert response.content == "User prefers dark mode."

    def test_get_context_validation_error(self) -> None:
        """Test validation error for missing query."""
        client = MAIAClient(base_url=BASE_URL)
        with pytest.raises(ValidationError) as exc_info:
            client.get_context(GetContextInput(query=""))
        assert exc_info.value.field == "query"

    @respx.mock
    def test_remember(self) -> None:
        """Test remember convenience method."""
        respx.post(f"{BASE_URL}/v1/memories").mock(
            return_value=Response(
                201,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "User likes coffee",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 0,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        memory = client.remember("test", "User likes coffee")

        assert memory.id == "mem-123"
        assert memory.content == "User likes coffee"

    @respx.mock
    def test_recall(self) -> None:
        """Test recall convenience method."""
        respx.post(f"{BASE_URL}/v1/context").mock(
            return_value=Response(
                200,
                json={
                    "content": "User likes coffee",
                    "memories": [],
                    "token_count": 5,
                    "token_budget": 1000,
                    "truncated": False,
                    "query_time": "1ms",
                },
            )
        )

        client = MAIAClient(base_url=BASE_URL)
        response = client.recall(
            "user preferences", namespace="test", token_budget=1000
        )

        assert response.content == "User likes coffee"

    @respx.mock
    def test_forget(self) -> None:
        """Test forget convenience method."""
        respx.delete(f"{BASE_URL}/v1/memories/mem-123").mock(
            return_value=Response(200, json={"deleted": True})
        )

        client = MAIAClient(base_url=BASE_URL)
        client.forget("mem-123")  # Should not raise

    def test_context_manager(self) -> None:
        """Test using client as context manager."""
        with MAIAClient(base_url=BASE_URL) as client:
            assert client is not None


class TestAsyncMAIAClient:
    """Tests for the asynchronous MAIA client."""

    @respx.mock
    @pytest.mark.asyncio
    async def test_health(self) -> None:
        """Test health check."""
        respx.get(f"{BASE_URL}/health").mock(
            return_value=Response(200, json={"status": "healthy", "service": "maia"})
        )

        async with AsyncMAIAClient(base_url=BASE_URL) as client:
            response = await client.health()

        assert response.status == "healthy"

    @respx.mock
    @pytest.mark.asyncio
    async def test_create_memory(self) -> None:
        """Test creating a memory."""
        respx.post(f"{BASE_URL}/v1/memories").mock(
            return_value=Response(
                201,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "Test memory",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 0,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        async with AsyncMAIAClient(base_url=BASE_URL) as client:
            memory = await client.create_memory(
                CreateMemoryInput(namespace="test", content="Test memory")
            )

        assert memory.id == "mem-123"

    @respx.mock
    @pytest.mark.asyncio
    async def test_remember(self) -> None:
        """Test remember convenience method."""
        respx.post(f"{BASE_URL}/v1/memories").mock(
            return_value=Response(
                201,
                json={
                    "id": "mem-123",
                    "namespace": "test",
                    "content": "User likes coffee",
                    "type": "semantic",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                    "accessed_at": "2024-01-01T00:00:00Z",
                    "access_count": 0,
                    "confidence": 1.0,
                    "source": "user",
                },
            )
        )

        async with AsyncMAIAClient(base_url=BASE_URL) as client:
            memory = await client.remember("test", "User likes coffee")

        assert memory.content == "User likes coffee"

    @respx.mock
    @pytest.mark.asyncio
    async def test_recall(self) -> None:
        """Test recall convenience method."""
        respx.post(f"{BASE_URL}/v1/context").mock(
            return_value=Response(
                200,
                json={
                    "content": "User likes coffee",
                    "memories": [],
                    "token_count": 5,
                    "token_budget": 1000,
                    "truncated": False,
                    "query_time": "1ms",
                },
            )
        )

        async with AsyncMAIAClient(base_url=BASE_URL) as client:
            response = await client.recall("user preferences")

        assert response.content == "User likes coffee"
