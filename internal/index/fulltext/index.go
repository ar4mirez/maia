// Package fulltext provides full-text search functionality for MAIA using Bleve.
package fulltext

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
)

// Common errors for full-text index operations.
var (
	ErrIndexClosed = errors.New("fulltext index is closed")
	ErrNotFound    = errors.New("document not found")
)

// Index defines the interface for full-text search.
type Index interface {
	// Index adds or updates a document in the index.
	Index(ctx context.Context, id string, doc *Document) error

	// IndexBatch adds or updates multiple documents in the index.
	IndexBatch(ctx context.Context, docs map[string]*Document) error

	// Delete removes a document from the index.
	Delete(ctx context.Context, id string) error

	// Search performs a full-text search query.
	Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResults, error)

	// Get retrieves a document by ID (if stored).
	Get(ctx context.Context, id string) (*Document, error)

	// Size returns the number of documents in the index.
	Size() (uint64, error)

	// Close releases resources held by the index.
	Close() error
}

// Document represents a document to be indexed.
type Document struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Namespace string                 `json:"namespace"`
	Tags      []string               `json:"tags,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SearchOptions configures a search query.
type SearchOptions struct {
	// Limit is the maximum number of results to return.
	Limit int

	// Offset is the number of results to skip.
	Offset int

	// Namespace filters results by namespace.
	Namespace string

	// Tags filters results by tags (AND logic).
	Tags []string

	// Type filters results by memory type.
	Type string

	// HighlightFields specifies which fields to highlight.
	HighlightFields []string
}

// DefaultSearchOptions returns default search options.
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:  10,
		Offset: 0,
	}
}

// SearchResults contains search results.
type SearchResults struct {
	Total    uint64
	Hits     []*SearchHit
	MaxScore float64
}

// SearchHit represents a single search result.
type SearchHit struct {
	ID         string
	Score      float64
	Highlights map[string][]string
}

// BleveIndex implements Index using Bleve.
type BleveIndex struct {
	index  bleve.Index
	path   string
	mu     sync.RWMutex
	closed bool
}

// Config holds configuration for the Bleve index.
type Config struct {
	// Path is the directory path for the index storage.
	// If empty, an in-memory index is created.
	Path string

	// InMemory creates an in-memory index (useful for testing).
	InMemory bool
}

// NewBleveIndex creates a new Bleve full-text index.
func NewBleveIndex(cfg Config) (*BleveIndex, error) {
	indexMapping := buildIndexMapping()

	var idx bleve.Index
	var err error

	if cfg.InMemory || cfg.Path == "" {
		idx, err = bleve.NewMemOnly(indexMapping)
	} else {
		// Try to open existing index
		idx, err = bleve.Open(cfg.Path)
		if err == bleve.ErrorIndexPathDoesNotExist {
			// Create new index
			idx, err = bleve.New(cfg.Path, indexMapping)
		}
	}

	if err != nil {
		return nil, err
	}

	return &BleveIndex{
		index: idx,
		path:  cfg.Path,
	}, nil
}

// buildIndexMapping creates the Bleve index mapping for documents.
func buildIndexMapping() *mapping.IndexMappingImpl {
	// Create a text field mapping with standard analyzer
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Analyzer = standard.Name

	// Create a keyword field mapping (not analyzed)
	keywordFieldMapping := bleve.NewKeywordFieldMapping()

	// Create the document mapping
	docMapping := bleve.NewDocumentMapping()
	docMapping.AddFieldMappingsAt("content", textFieldMapping)
	docMapping.AddFieldMappingsAt("namespace", keywordFieldMapping)
	docMapping.AddFieldMappingsAt("tags", keywordFieldMapping)
	docMapping.AddFieldMappingsAt("type", keywordFieldMapping)

	// Create the index mapping
	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultMapping = docMapping
	indexMapping.DefaultAnalyzer = standard.Name

	return indexMapping
}

// Index adds or updates a document in the index.
func (idx *BleveIndex) Index(ctx context.Context, id string, doc *Document) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	doc.ID = id
	return idx.index.Index(id, doc)
}

// IndexBatch adds or updates multiple documents in the index.
func (idx *BleveIndex) IndexBatch(ctx context.Context, docs map[string]*Document) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	batch := idx.index.NewBatch()
	for id, doc := range docs {
		doc.ID = id
		if err := batch.Index(id, doc); err != nil {
			return err
		}
	}

	return idx.index.Batch(batch)
}

// Delete removes a document from the index.
func (idx *BleveIndex) Delete(ctx context.Context, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return ErrIndexClosed
	}

	return idx.index.Delete(id)
}

// Search performs a full-text search query.
func (idx *BleveIndex) Search(ctx context.Context, queryStr string, opts *SearchOptions) (*SearchResults, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	if opts == nil {
		opts = DefaultSearchOptions()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Build the query
	query := idx.buildQuery(queryStr, opts)

	// Create search request
	searchRequest := bleve.NewSearchRequestOptions(query, opts.Limit, opts.Offset, false)

	// Add highlighting if requested
	if len(opts.HighlightFields) > 0 {
		searchRequest.Highlight = bleve.NewHighlight()
		for _, field := range opts.HighlightFields {
			searchRequest.Highlight.AddField(field)
		}
	}

	// Execute search
	searchResult, err := idx.index.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	// Convert results
	results := &SearchResults{
		Total:    searchResult.Total,
		MaxScore: searchResult.MaxScore,
		Hits:     make([]*SearchHit, len(searchResult.Hits)),
	}

	for i, hit := range searchResult.Hits {
		results.Hits[i] = &SearchHit{
			ID:         hit.ID,
			Score:      hit.Score,
			Highlights: make(map[string][]string),
		}

		// Copy highlights
		for field, fragments := range hit.Fragments {
			results.Hits[i].Highlights[field] = fragments
		}
	}

	return results, nil
}

// buildQuery constructs a Bleve query from the search string and options.
func (idx *BleveIndex) buildQuery(queryStr string, opts *SearchOptions) query.Query {
	queries := make([]query.Query, 0)

	// Main content query
	if queryStr != "" {
		contentQuery := bleve.NewMatchQuery(queryStr)
		contentQuery.SetField("content")
		queries = append(queries, contentQuery)
	}

	// Namespace filter
	if opts.Namespace != "" {
		nsQuery := bleve.NewTermQuery(opts.Namespace)
		nsQuery.SetField("namespace")
		queries = append(queries, nsQuery)
	}

	// Type filter
	if opts.Type != "" {
		typeQuery := bleve.NewTermQuery(opts.Type)
		typeQuery.SetField("type")
		queries = append(queries, typeQuery)
	}

	// Tags filter (AND logic - all tags must match)
	for _, tag := range opts.Tags {
		tagQuery := bleve.NewTermQuery(tag)
		tagQuery.SetField("tags")
		queries = append(queries, tagQuery)
	}

	// Combine queries
	if len(queries) == 0 {
		return bleve.NewMatchAllQuery()
	}

	if len(queries) == 1 {
		return queries[0]
	}

	return bleve.NewConjunctionQuery(queries...)
}

// Get retrieves a document by ID.
func (idx *BleveIndex) Get(ctx context.Context, id string) (*Document, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return nil, ErrIndexClosed
	}

	doc, err := idx.index.Document(id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrNotFound
	}

	// Note: Bleve doesn't store the original document by default
	// This returns minimal info. For full document retrieval,
	// use the storage layer.
	return &Document{ID: id}, nil
}

// Size returns the number of documents in the index.
func (idx *BleveIndex) Size() (uint64, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.closed {
		return 0, ErrIndexClosed
	}

	return idx.index.DocCount()
}

// Close closes the index.
func (idx *BleveIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return nil
	}

	idx.closed = true
	return idx.index.Close()
}

// DeleteIndex removes the index files from disk.
func DeleteIndex(path string) error {
	return os.RemoveAll(path)
}
