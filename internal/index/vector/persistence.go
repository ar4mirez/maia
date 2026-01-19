// Package vector provides vector similarity search functionality for MAIA.
package vector

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Persistence errors.
var (
	ErrInvalidFormat  = errors.New("invalid index format")
	ErrVersionMismatch = errors.New("index version mismatch")
)

// Index file format constants.
const (
	magicNumber    = 0x4D414941 // "MAIA" in ASCII
	formatVersion  = 1
	hnswIndexType  = 1
	bruteIndexType = 2
)

// PersistentIndex extends Index with persistence capabilities.
type PersistentIndex interface {
	Index
	// Save persists the index to the specified path.
	Save(path string) error
	// Load restores the index from the specified path.
	Load(path string) error
}

// Save persists the HNSW index to disk.
func (idx *HNSWIndex) Save(path string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return ErrIndexClosed
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create index file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	// Write header
	if err := idx.writeHeader(w); err != nil {
		return err
	}

	// Write HNSW-specific metadata
	if err := binary.Write(w, binary.LittleEndian, int32(idx.m)); err != nil {
		return fmt.Errorf("failed to write M: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(idx.mMax)); err != nil {
		return fmt.Errorf("failed to write mMax: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(idx.efConstruction)); err != nil {
		return fmt.Errorf("failed to write efConstruction: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(idx.efSearch)); err != nil {
		return fmt.Errorf("failed to write efSearch: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, idx.levelMult); err != nil {
		return fmt.Errorf("failed to write levelMult: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(idx.maxLevel)); err != nil {
		return fmt.Errorf("failed to write maxLevel: %w", err)
	}

	// Write entry node ID (or empty string if nil)
	entryID := ""
	if idx.entryNode != nil {
		entryID = idx.entryNode.id
	}
	if err := writeString(w, entryID); err != nil {
		return fmt.Errorf("failed to write entry node ID: %w", err)
	}

	// Write nodes
	if err := binary.Write(w, binary.LittleEndian, int32(len(idx.nodes))); err != nil {
		return fmt.Errorf("failed to write node count: %w", err)
	}

	for _, node := range idx.nodes {
		if err := idx.writeNode(w, node); err != nil {
			return err
		}
	}

	return w.Flush()
}

// writeHeader writes the common index header.
func (idx *HNSWIndex) writeHeader(w io.Writer) error {
	// Magic number
	if err := binary.Write(w, binary.LittleEndian, uint32(magicNumber)); err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}
	// Version
	if err := binary.Write(w, binary.LittleEndian, uint32(formatVersion)); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}
	// Index type
	if err := binary.Write(w, binary.LittleEndian, uint32(hnswIndexType)); err != nil {
		return fmt.Errorf("failed to write index type: %w", err)
	}
	// Dimension
	if err := binary.Write(w, binary.LittleEndian, int32(idx.dimension)); err != nil {
		return fmt.Errorf("failed to write dimension: %w", err)
	}
	return nil
}

// writeNode writes a single HNSW node to the writer.
func (idx *HNSWIndex) writeNode(w io.Writer, node *hnswNode) error {
	// Write ID
	if err := writeString(w, node.id); err != nil {
		return fmt.Errorf("failed to write node ID: %w", err)
	}

	// Write level
	if err := binary.Write(w, binary.LittleEndian, int32(node.level)); err != nil {
		return fmt.Errorf("failed to write node level: %w", err)
	}

	// Write vector
	if err := binary.Write(w, binary.LittleEndian, int32(len(node.vector))); err != nil {
		return fmt.Errorf("failed to write vector length: %w", err)
	}
	for _, v := range node.vector {
		if err := binary.Write(w, binary.LittleEndian, v); err != nil {
			return fmt.Errorf("failed to write vector element: %w", err)
		}
	}

	// Write neighbors for each level
	if err := binary.Write(w, binary.LittleEndian, int32(len(node.neighbors))); err != nil {
		return fmt.Errorf("failed to write neighbor levels count: %w", err)
	}
	for l := 0; l < len(node.neighbors); l++ {
		if err := binary.Write(w, binary.LittleEndian, int32(len(node.neighbors[l]))); err != nil {
			return fmt.Errorf("failed to write neighbor count at level %d: %w", l, err)
		}
		for neighborID := range node.neighbors[l] {
			if err := writeString(w, neighborID); err != nil {
				return fmt.Errorf("failed to write neighbor ID: %w", err)
			}
		}
	}

	return nil
}

// Load restores the HNSW index from disk.
func (idx *HNSWIndex) Load(path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)

	// Read and validate header
	dimension, err := idx.readHeader(r)
	if err != nil {
		return err
	}
	idx.dimension = dimension

	// Read HNSW-specific metadata
	var m, mMax, efConstruction, efSearch, maxLevel int32
	var levelMult float64

	if err := binary.Read(r, binary.LittleEndian, &m); err != nil {
		return fmt.Errorf("failed to read M: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &mMax); err != nil {
		return fmt.Errorf("failed to read mMax: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &efConstruction); err != nil {
		return fmt.Errorf("failed to read efConstruction: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &efSearch); err != nil {
		return fmt.Errorf("failed to read efSearch: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &levelMult); err != nil {
		return fmt.Errorf("failed to read levelMult: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &maxLevel); err != nil {
		return fmt.Errorf("failed to read maxLevel: %w", err)
	}

	idx.m = int(m)
	idx.mMax = int(mMax)
	idx.efConstruction = int(efConstruction)
	idx.efSearch = int(efSearch)
	idx.levelMult = levelMult
	idx.maxLevel = int(maxLevel)

	// Read entry node ID
	entryID, err := readString(r)
	if err != nil {
		return fmt.Errorf("failed to read entry node ID: %w", err)
	}

	// Read node count
	var nodeCount int32
	if err := binary.Read(r, binary.LittleEndian, &nodeCount); err != nil {
		return fmt.Errorf("failed to read node count: %w", err)
	}

	// Read nodes (first pass: create nodes without neighbor links)
	idx.nodes = make(map[string]*hnswNode, nodeCount)
	nodeNeighborIDs := make(map[string][][]string, nodeCount) // Store neighbor IDs for second pass

	for i := int32(0); i < nodeCount; i++ {
		node, neighborIDs, err := idx.readNode(r)
		if err != nil {
			return err
		}
		idx.nodes[node.id] = node
		nodeNeighborIDs[node.id] = neighborIDs
	}

	// Second pass: link neighbors
	for nodeID, neighborIDsByLevel := range nodeNeighborIDs {
		node := idx.nodes[nodeID]
		for level, neighborIDs := range neighborIDsByLevel {
			for _, neighborID := range neighborIDs {
				if neighbor, exists := idx.nodes[neighborID]; exists {
					node.neighbors[level][neighborID] = neighbor
				}
			}
		}
	}

	// Set entry node
	if entryID != "" {
		idx.entryNode = idx.nodes[entryID]
	}

	idx.closed = false
	return nil
}

// readHeader reads and validates the common index header.
func (idx *HNSWIndex) readHeader(r io.Reader) (int, error) {
	var magic, version, indexType uint32
	var dimension int32

	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return 0, fmt.Errorf("failed to read magic number: %w", err)
	}
	if magic != magicNumber {
		return 0, ErrInvalidFormat
	}

	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return 0, fmt.Errorf("failed to read version: %w", err)
	}
	if version != formatVersion {
		return 0, fmt.Errorf("%w: expected %d, got %d", ErrVersionMismatch, formatVersion, version)
	}

	if err := binary.Read(r, binary.LittleEndian, &indexType); err != nil {
		return 0, fmt.Errorf("failed to read index type: %w", err)
	}
	if indexType != hnswIndexType {
		return 0, fmt.Errorf("%w: expected HNSW index", ErrInvalidFormat)
	}

	if err := binary.Read(r, binary.LittleEndian, &dimension); err != nil {
		return 0, fmt.Errorf("failed to read dimension: %w", err)
	}

	return int(dimension), nil
}

// readNode reads a single HNSW node from the reader.
// Returns the node and neighbor IDs for each level (to be linked in second pass).
func (idx *HNSWIndex) readNode(r io.Reader) (*hnswNode, [][]string, error) {
	// Read ID
	id, err := readString(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read node ID: %w", err)
	}

	// Read level
	var level int32
	if err := binary.Read(r, binary.LittleEndian, &level); err != nil {
		return nil, nil, fmt.Errorf("failed to read node level: %w", err)
	}

	// Read vector
	var vectorLen int32
	if err := binary.Read(r, binary.LittleEndian, &vectorLen); err != nil {
		return nil, nil, fmt.Errorf("failed to read vector length: %w", err)
	}
	vector := make([]float32, vectorLen)
	for i := range vector {
		if err := binary.Read(r, binary.LittleEndian, &vector[i]); err != nil {
			return nil, nil, fmt.Errorf("failed to read vector element: %w", err)
		}
	}

	// Read neighbor level count
	var neighborLevels int32
	if err := binary.Read(r, binary.LittleEndian, &neighborLevels); err != nil {
		return nil, nil, fmt.Errorf("failed to read neighbor levels count: %w", err)
	}

	// Create node with empty neighbors
	node := &hnswNode{
		id:        id,
		vector:    vector,
		level:     int(level),
		neighbors: make([]map[string]*hnswNode, neighborLevels),
	}
	for i := range node.neighbors {
		node.neighbors[i] = make(map[string]*hnswNode)
	}

	// Read neighbor IDs for each level
	neighborIDsByLevel := make([][]string, neighborLevels)
	for l := int32(0); l < neighborLevels; l++ {
		var neighborCount int32
		if err := binary.Read(r, binary.LittleEndian, &neighborCount); err != nil {
			return nil, nil, fmt.Errorf("failed to read neighbor count at level %d: %w", l, err)
		}
		neighborIDsByLevel[l] = make([]string, neighborCount)
		for i := int32(0); i < neighborCount; i++ {
			neighborID, err := readString(r)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read neighbor ID: %w", err)
			}
			neighborIDsByLevel[l][i] = neighborID
		}
	}

	return node, neighborIDsByLevel, nil
}

// Save persists the BruteForce index to disk.
func (idx *BruteForceIndex) Save(path string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return ErrIndexClosed
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create index file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	// Write header
	if err := binary.Write(w, binary.LittleEndian, uint32(magicNumber)); err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(formatVersion)); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(bruteIndexType)); err != nil {
		return fmt.Errorf("failed to write index type: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(idx.dimension)); err != nil {
		return fmt.Errorf("failed to write dimension: %w", err)
	}

	// Write vector count
	if err := binary.Write(w, binary.LittleEndian, int32(len(idx.vectors))); err != nil {
		return fmt.Errorf("failed to write vector count: %w", err)
	}

	// Write each vector
	for id, vector := range idx.vectors {
		if err := writeString(w, id); err != nil {
			return fmt.Errorf("failed to write vector ID: %w", err)
		}
		for _, v := range vector {
			if err := binary.Write(w, binary.LittleEndian, v); err != nil {
				return fmt.Errorf("failed to write vector element: %w", err)
			}
		}
	}

	return w.Flush()
}

// Load restores the BruteForce index from disk.
func (idx *BruteForceIndex) Load(path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)

	// Read and validate header
	var magic, version, indexType uint32
	var dimension int32

	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("failed to read magic number: %w", err)
	}
	if magic != magicNumber {
		return ErrInvalidFormat
	}

	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if version != formatVersion {
		return fmt.Errorf("%w: expected %d, got %d", ErrVersionMismatch, formatVersion, version)
	}

	if err := binary.Read(r, binary.LittleEndian, &indexType); err != nil {
		return fmt.Errorf("failed to read index type: %w", err)
	}
	if indexType != bruteIndexType {
		return fmt.Errorf("%w: expected BruteForce index", ErrInvalidFormat)
	}

	if err := binary.Read(r, binary.LittleEndian, &dimension); err != nil {
		return fmt.Errorf("failed to read dimension: %w", err)
	}
	idx.dimension = int(dimension)

	// Read vector count
	var vectorCount int32
	if err := binary.Read(r, binary.LittleEndian, &vectorCount); err != nil {
		return fmt.Errorf("failed to read vector count: %w", err)
	}

	// Read vectors
	idx.vectors = make(map[string][]float32, vectorCount)
	for i := int32(0); i < vectorCount; i++ {
		id, err := readString(r)
		if err != nil {
			return fmt.Errorf("failed to read vector ID: %w", err)
		}
		vector := make([]float32, dimension)
		for j := range vector {
			if err := binary.Read(r, binary.LittleEndian, &vector[j]); err != nil {
				return fmt.Errorf("failed to read vector element: %w", err)
			}
		}
		idx.vectors[id] = vector
	}

	idx.closed = false
	return nil
}

// writeString writes a length-prefixed string.
func writeString(w io.Writer, s string) error {
	bytes := []byte(s)
	if err := binary.Write(w, binary.LittleEndian, int32(len(bytes))); err != nil {
		return err
	}
	_, err := w.Write(bytes)
	return err
}

// readString reads a length-prefixed string.
func readString(r io.Reader) (string, error) {
	var length int32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	bytes := make([]byte, length)
	if _, err := io.ReadFull(r, bytes); err != nil {
		return "", err
	}
	return string(bytes), nil
}

// LoadHNSWIndex loads an HNSW index from the specified path.
func LoadHNSWIndex(path string) (*HNSWIndex, error) {
	idx := &HNSWIndex{
		nodes: make(map[string]*hnswNode),
	}
	if err := idx.Load(path); err != nil {
		return nil, err
	}
	return idx, nil
}

// LoadBruteForceIndex loads a BruteForce index from the specified path.
func LoadBruteForceIndex(path string) (*BruteForceIndex, error) {
	idx := &BruteForceIndex{
		vectors: make(map[string][]float32),
	}
	if err := idx.Load(path); err != nil {
		return nil, err
	}
	return idx, nil
}
