package graph

import (
	"encoding/binary"
	"errors"
	"io"
	"time"
)

// Persistence format constants.
const (
	magicNumber = 0x4D414947 // "MAIG" (MAIA Graph)
	version     = 1
)

// Persistence errors.
var (
	ErrInvalidFormat  = errors.New("invalid graph index format")
	ErrVersionMismatch = errors.New("graph index version mismatch")
)

// saveIndex writes the graph index to a writer in binary format.
func saveIndex(w io.Writer, outgoing map[string][]Edge) error {
	// Write magic number
	if err := binary.Write(w, binary.LittleEndian, uint32(magicNumber)); err != nil {
		return err
	}

	// Write version
	if err := binary.Write(w, binary.LittleEndian, uint16(version)); err != nil {
		return err
	}

	// Count total edges
	var edgeCount uint32
	for _, edges := range outgoing {
		edgeCount += uint32(len(edges))
	}

	// Write edge count
	if err := binary.Write(w, binary.LittleEndian, edgeCount); err != nil {
		return err
	}

	// Write each edge
	for _, edges := range outgoing {
		for _, edge := range edges {
			if err := writeEdge(w, &edge); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeEdge writes a single edge to the writer.
func writeEdge(w io.Writer, edge *Edge) error {
	// Write source ID
	if err := writeString(w, edge.SourceID); err != nil {
		return err
	}

	// Write target ID
	if err := writeString(w, edge.TargetID); err != nil {
		return err
	}

	// Write relation
	if err := writeString(w, edge.Relation); err != nil {
		return err
	}

	// Write weight
	if err := binary.Write(w, binary.LittleEndian, edge.Weight); err != nil {
		return err
	}

	// Write metadata count
	metaCount := uint16(len(edge.Metadata))
	if err := binary.Write(w, binary.LittleEndian, metaCount); err != nil {
		return err
	}

	// Write metadata entries
	for k, v := range edge.Metadata {
		if err := writeString(w, k); err != nil {
			return err
		}
		if err := writeString(w, v); err != nil {
			return err
		}
	}

	// Write created timestamp
	if err := binary.Write(w, binary.LittleEndian, edge.CreatedAt.UnixNano()); err != nil {
		return err
	}

	return nil
}

// writeString writes a length-prefixed string.
func writeString(w io.Writer, s string) error {
	data := []byte(s)
	if err := binary.Write(w, binary.LittleEndian, uint16(len(data))); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// loadIndex reads a graph index from a reader.
func loadIndex(r io.Reader) (map[string][]Edge, error) {
	// Read and validate magic number
	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != magicNumber {
		return nil, ErrInvalidFormat
	}

	// Read and validate version
	var ver uint16
	if err := binary.Read(r, binary.LittleEndian, &ver); err != nil {
		return nil, err
	}
	if ver != version {
		return nil, ErrVersionMismatch
	}

	// Read edge count
	var edgeCount uint32
	if err := binary.Read(r, binary.LittleEndian, &edgeCount); err != nil {
		return nil, err
	}

	// Read edges
	outgoing := make(map[string][]Edge)
	for i := uint32(0); i < edgeCount; i++ {
		edge, err := readEdge(r)
		if err != nil {
			return nil, err
		}
		outgoing[edge.SourceID] = append(outgoing[edge.SourceID], *edge)
	}

	return outgoing, nil
}

// readEdge reads a single edge from the reader.
func readEdge(r io.Reader) (*Edge, error) {
	edge := &Edge{}

	// Read source ID
	sourceID, err := readString(r)
	if err != nil {
		return nil, err
	}
	edge.SourceID = sourceID

	// Read target ID
	targetID, err := readString(r)
	if err != nil {
		return nil, err
	}
	edge.TargetID = targetID

	// Read relation
	relation, err := readString(r)
	if err != nil {
		return nil, err
	}
	edge.Relation = relation

	// Read weight
	if err := binary.Read(r, binary.LittleEndian, &edge.Weight); err != nil {
		return nil, err
	}

	// Read metadata count
	var metaCount uint16
	if err := binary.Read(r, binary.LittleEndian, &metaCount); err != nil {
		return nil, err
	}

	// Read metadata entries
	if metaCount > 0 {
		edge.Metadata = make(map[string]string, metaCount)
		for i := uint16(0); i < metaCount; i++ {
			key, err := readString(r)
			if err != nil {
				return nil, err
			}
			value, err := readString(r)
			if err != nil {
				return nil, err
			}
			edge.Metadata[key] = value
		}
	}

	// Read created timestamp
	var nanos int64
	if err := binary.Read(r, binary.LittleEndian, &nanos); err != nil {
		return nil, err
	}
	edge.CreatedAt = time.Unix(0, nanos)

	return edge, nil
}

// readString reads a length-prefixed string.
func readString(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}
