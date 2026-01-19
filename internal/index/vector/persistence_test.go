package vector

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHNSWIndex_SaveLoad_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Create empty index
	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)

	// Save
	err := idx.Save(path)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load into new index
	loaded, err := LoadHNSWIndex(path)
	require.NoError(t, err)

	assert.Equal(t, idx.dimension, loaded.dimension)
	assert.Equal(t, idx.m, loaded.m)
	assert.Equal(t, idx.mMax, loaded.mMax)
	assert.Equal(t, idx.efConstruction, loaded.efConstruction)
	assert.Equal(t, idx.efSearch, loaded.efSearch)
	assert.Equal(t, 0, loaded.Size())
}

func TestHNSWIndex_SaveLoad_WithVectors(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")
	ctx := context.Background()

	// Create index with vectors
	cfg := DefaultConfig(4)
	idx := NewHNSWIndex(cfg)

	vectors := map[string][]float32{
		"vec1": {1.0, 0.0, 0.0, 0.0},
		"vec2": {0.0, 1.0, 0.0, 0.0},
		"vec3": {0.0, 0.0, 1.0, 0.0},
		"vec4": {0.5, 0.5, 0.0, 0.0},
		"vec5": {0.0, 0.5, 0.5, 0.0},
	}

	for id, vec := range vectors {
		err := idx.Add(ctx, id, vec)
		require.NoError(t, err)
	}

	// Save
	err := idx.Save(path)
	require.NoError(t, err)

	// Load into new index
	loaded, err := LoadHNSWIndex(path)
	require.NoError(t, err)

	// Verify size
	assert.Equal(t, len(vectors), loaded.Size())

	// Verify all vectors can be retrieved
	for id, expectedVec := range vectors {
		vec, err := loaded.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, expectedVec, vec, "vector %s mismatch", id)
	}

	// Verify search works
	results, err := loaded.Search(ctx, []float32{1.0, 0.0, 0.0, 0.0}, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "vec1", results[0].ID)
}

func TestHNSWIndex_SaveLoad_PreservesNeighborLinks(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")
	ctx := context.Background()

	// Create index with enough vectors to establish neighbor relationships
	cfg := Config{
		Dimension:      4,
		M:              4,
		EfConstruction: 50,
		EfSearch:       20,
	}
	idx := NewHNSWIndex(cfg)

	// Add vectors with predictable IDs
	vectors := map[string][]float32{
		"alpha":   {1.0, 0.0, 0.0, 0.0},
		"beta":    {0.9, 0.1, 0.0, 0.0},
		"gamma":   {0.8, 0.2, 0.0, 0.0},
		"delta":   {0.0, 1.0, 0.0, 0.0},
		"epsilon": {0.0, 0.9, 0.1, 0.0},
		"zeta":    {0.0, 0.0, 1.0, 0.0},
		"eta":     {0.0, 0.0, 0.0, 1.0},
		"theta":   {0.5, 0.5, 0.0, 0.0},
		"iota":    {0.0, 0.5, 0.5, 0.0},
		"kappa":   {0.25, 0.25, 0.25, 0.25},
	}
	for id, vec := range vectors {
		err := idx.Add(ctx, id, vec)
		require.NoError(t, err)
	}

	// Save and load
	err := idx.Save(path)
	require.NoError(t, err)

	loaded, err := LoadHNSWIndex(path)
	require.NoError(t, err)

	// Verify all vectors can be retrieved and match
	for id, expectedVec := range vectors {
		vec, err := loaded.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, expectedVec, vec, "vector %s mismatch", id)
	}

	// Verify search returns same result count
	query := []float32{1.0, 0.0, 0.0, 0.0}
	origResults, err := idx.Search(ctx, query, 3)
	require.NoError(t, err)

	loadedResults, err := loaded.Search(ctx, query, 3)
	require.NoError(t, err)

	assert.Equal(t, len(origResults), len(loadedResults))
	// Top result should be "alpha" as it's identical to the query
	assert.Equal(t, "alpha", loadedResults[0].ID)
}

func TestHNSWIndex_Save_ClosedIndex(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)
	_ = idx.Close()

	err := idx.Save(path)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestHNSWIndex_Load_FileNotFound(t *testing.T) {
	_, err := LoadHNSWIndex("/nonexistent/path/index.bin")
	assert.Error(t, err)
}

func TestHNSWIndex_Load_InvalidMagicNumber(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Write invalid magic number
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = binary.Write(f, binary.LittleEndian, uint32(0x12345678))
	f.Close()

	_, err = LoadHNSWIndex(path)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestHNSWIndex_Load_VersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Write valid magic but wrong version
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = binary.Write(f, binary.LittleEndian, uint32(magicNumber))
	_ = binary.Write(f, binary.LittleEndian, uint32(999)) // Invalid version
	f.Close()

	_, err = LoadHNSWIndex(path)
	assert.ErrorIs(t, err, ErrVersionMismatch)
}

func TestHNSWIndex_Load_WrongIndexType(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Write valid header but wrong index type
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = binary.Write(f, binary.LittleEndian, uint32(magicNumber))
	_ = binary.Write(f, binary.LittleEndian, uint32(formatVersion))
	_ = binary.Write(f, binary.LittleEndian, uint32(bruteIndexType)) // Wrong type
	f.Close()

	_, err = LoadHNSWIndex(path)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestHNSWIndex_Save_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "deep", "index.bin")

	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)

	err := idx.Save(path)
	require.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(path))
	require.NoError(t, err)
}

func TestBruteForceIndex_SaveLoad_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Create empty index
	idx := NewBruteForceIndex(128)

	// Save
	err := idx.Save(path)
	require.NoError(t, err)

	// Load into new index
	loaded, err := LoadBruteForceIndex(path)
	require.NoError(t, err)

	assert.Equal(t, idx.dimension, loaded.dimension)
	assert.Equal(t, 0, loaded.Size())
}

func TestBruteForceIndex_SaveLoad_WithVectors(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")
	ctx := context.Background()

	// Create index with vectors
	idx := NewBruteForceIndex(4)

	vectors := map[string][]float32{
		"vec1": {1.0, 0.0, 0.0, 0.0},
		"vec2": {0.0, 1.0, 0.0, 0.0},
		"vec3": {0.0, 0.0, 1.0, 0.0},
	}

	for id, vec := range vectors {
		err := idx.Add(ctx, id, vec)
		require.NoError(t, err)
	}

	// Save
	err := idx.Save(path)
	require.NoError(t, err)

	// Load into new index
	loaded, err := LoadBruteForceIndex(path)
	require.NoError(t, err)

	// Verify size
	assert.Equal(t, len(vectors), loaded.Size())

	// Verify all vectors can be retrieved
	for id, expectedVec := range vectors {
		vec, err := loaded.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, expectedVec, vec, "vector %s mismatch", id)
	}

	// Verify search works
	results, err := loaded.Search(ctx, []float32{1.0, 0.0, 0.0, 0.0}, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "vec1", results[0].ID)
}

func TestBruteForceIndex_Save_ClosedIndex(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	idx := NewBruteForceIndex(128)
	_ = idx.Close()

	err := idx.Save(path)
	assert.ErrorIs(t, err, ErrIndexClosed)
}

func TestBruteForceIndex_Load_FileNotFound(t *testing.T) {
	_, err := LoadBruteForceIndex("/nonexistent/path/index.bin")
	assert.Error(t, err)
}

func TestBruteForceIndex_Load_InvalidMagicNumber(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Write invalid magic number
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = binary.Write(f, binary.LittleEndian, uint32(0x12345678))
	f.Close()

	_, err = LoadBruteForceIndex(path)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestBruteForceIndex_Load_WrongIndexType(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")

	// Write valid header but wrong index type
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = binary.Write(f, binary.LittleEndian, uint32(magicNumber))
	_ = binary.Write(f, binary.LittleEndian, uint32(formatVersion))
	_ = binary.Write(f, binary.LittleEndian, uint32(hnswIndexType)) // Wrong type
	f.Close()

	_, err = LoadBruteForceIndex(path)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestWriteReadString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short", "hello"},
		{"long", "this is a longer string with various characters: !@#$%^&*()"},
		{"unicode", "hello 世界 🌍"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}

			err := writeString(buf, tt.input)
			require.NoError(t, err)

			result, err := readString(buf)
			require.NoError(t, err)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestHNSWIndex_SaveLoad_LargeIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large index test in short mode")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "index.bin")
	ctx := context.Background()

	// Create index with many vectors
	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)

	numVectors := 1000
	vectors := make(map[string][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		id := string(rune('a' + (i % 26)))
		if i >= 26 {
			id = string(rune('a'+(i%26))) + string(rune('0'+(i/26)))
		}
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = float32(i*j) / float32(numVectors*128)
		}
		vectors[id] = vec
		err := idx.Add(ctx, id, vec)
		require.NoError(t, err)
	}

	// Save
	err := idx.Save(path)
	require.NoError(t, err)

	// Load
	loaded, err := LoadHNSWIndex(path)
	require.NoError(t, err)

	assert.Equal(t, numVectors, loaded.Size())
}

func TestPersistentIndex_Interface(t *testing.T) {
	// Verify that both index types implement PersistentIndex
	var _ PersistentIndex = &HNSWIndex{}
	var _ PersistentIndex = &BruteForceIndex{}
}

func BenchmarkHNSWIndex_Save(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := context.Background()

	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)

	// Add some vectors
	for i := 0; i < 100; i++ {
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = float32(i*j) / 12800.0
		}
		_ = idx.Add(ctx, string(rune('a'+i)), vec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := filepath.Join(tmpDir, "index.bin")
		_ = idx.Save(path)
	}
}

func BenchmarkHNSWIndex_Load(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "index.bin")
	ctx := context.Background()

	cfg := DefaultConfig(128)
	idx := NewHNSWIndex(cfg)

	// Add some vectors
	for i := 0; i < 100; i++ {
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = float32(i*j) / 12800.0
		}
		_ = idx.Add(ctx, string(rune('a'+i)), vec)
	}

	_ = idx.Save(path)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadHNSWIndex(path)
	}
}
