package replication

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
)

const (
	// WAL key prefixes
	walEntryPrefix    = "wal:"     // wal:{id} -> WALEntry
	walSequencePrefix = "walseq:"  // walseq:{sequence} -> entry ID
	walMetaKey        = "wal:meta" // metadata (current sequence, etc.)

	// Default batch size for reads
	defaultReadBatchSize = 100
)

// walMeta stores WAL metadata.
type walMeta struct {
	CurrentSequence uint64    `json:"current_sequence"`
	OldestSequence  uint64    `json:"oldest_sequence"`
	EntryCount      int64     `json:"entry_count"`
	TotalBytes      int64     `json:"total_bytes"`
	LastCompaction  time.Time `json:"last_compaction,omitempty"`
}

// BadgerWAL implements WAL using BadgerDB.
type BadgerWAL struct {
	db       *badger.DB
	logger   *zap.Logger
	region   string
	entropy  *ulid.MonotonicEntropy
	sequence atomic.Uint64
	mu       sync.RWMutex
	closed   atomic.Bool
}

// newEntropy creates a new monotonic entropy source for ULID generation.
func newEntropy() *ulid.MonotonicEntropy {
	return ulid.Monotonic(rand.Reader, 0)
}

// BadgerWALOptions configures the BadgerWAL.
type BadgerWALOptions struct {
	// DataDir is the directory for WAL data.
	DataDir string

	// Region is the region identifier for this WAL.
	Region string

	// Logger is the logger to use.
	Logger *zap.Logger

	// SyncWrites enables synchronous writes for durability.
	SyncWrites bool

	// ValueLogFileSize is the size of value log files.
	ValueLogFileSize int64
}

// NewBadgerWAL creates a new BadgerDB-backed WAL.
func NewBadgerWAL(opts *BadgerWALOptions) (*BadgerWAL, error) {
	if opts.DataDir == "" {
		return nil, errors.New("data directory is required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	badgerOpts := badger.DefaultOptions(opts.DataDir)
	badgerOpts.SyncWrites = opts.SyncWrites
	badgerOpts.Logger = nil // Disable Badger's default logging

	if opts.ValueLogFileSize > 0 {
		badgerOpts.ValueLogFileSize = opts.ValueLogFileSize
	} else {
		badgerOpts.ValueLogFileSize = 64 << 20 // 64MB default
	}

	db, err := badger.Open(badgerOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL database: %w", err)
	}

	w := &BadgerWAL{
		db:      db,
		logger:  logger,
		region:  opts.Region,
		entropy: newEntropy(),
	}

	// Load current sequence from metadata
	if err := w.loadMeta(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load WAL metadata: %w", err)
	}

	return w, nil
}

// loadMeta loads WAL metadata from the database.
func (w *BadgerWAL) loadMeta() error {
	return w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(walMetaKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			// No metadata yet, start fresh
			w.sequence.Store(0)
			return nil
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			var meta walMeta
			if err := json.Unmarshal(val, &meta); err != nil {
				return err
			}
			w.sequence.Store(meta.CurrentSequence)
			return nil
		})
	})
}

// saveMeta persists WAL metadata.
func (w *BadgerWAL) saveMeta(txn *badger.Txn, meta *walMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return txn.Set([]byte(walMetaKey), data)
}

// Append adds an entry to the WAL.
func (w *BadgerWAL) Append(ctx context.Context, entry *WALEntry) error {
	if w.closed.Load() {
		return ErrWALClosed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = ulid.MustNew(ulid.Timestamp(time.Now()), w.entropy).String()
	}

	// Assign sequence number
	entry.Sequence = w.sequence.Add(1)

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Set region
	if entry.Region == "" {
		entry.Region = w.region
	}

	// Compute checksum
	entry.Checksum = entry.ComputeChecksum()

	// Validate entry
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("invalid WAL entry: %w", err)
	}

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	// Write to database
	err = w.db.Update(func(txn *badger.Txn) error {
		// Store entry by ID
		entryKey := []byte(fmt.Sprintf("%s%s", walEntryPrefix, entry.ID))
		if err := txn.Set(entryKey, data); err != nil {
			return err
		}

		// Store sequence -> ID mapping
		seqKey := []byte(fmt.Sprintf("%s%020d", walSequencePrefix, entry.Sequence))
		if err := txn.Set(seqKey, []byte(entry.ID)); err != nil {
			return err
		}

		// Update metadata
		meta := &walMeta{
			CurrentSequence: entry.Sequence,
			EntryCount:      0, // Will be computed on read
			TotalBytes:      0, // Will be computed on read
		}
		return w.saveMeta(txn, meta)
	})

	if err != nil {
		return fmt.Errorf("failed to append WAL entry: %w", err)
	}

	w.logger.Debug("appended WAL entry",
		zap.String("id", entry.ID),
		zap.Uint64("sequence", entry.Sequence),
		zap.String("operation", string(entry.Operation)),
		zap.String("resource_type", string(entry.ResourceType)),
		zap.String("resource_id", entry.ResourceID),
	)

	return nil
}

// Read returns entries after the given sequence number.
func (w *BadgerWAL) Read(ctx context.Context, afterSequence uint64, limit int) ([]*WALEntry, error) {
	if w.closed.Load() {
		return nil, ErrWALClosed
	}

	if limit <= 0 {
		limit = defaultReadBatchSize
	}

	var entries []*WALEntry

	err := w.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = limit
		it := txn.NewIterator(opts)
		defer it.Close()

		// Start from the sequence after the given one
		startKey := []byte(fmt.Sprintf("%s%020d", walSequencePrefix, afterSequence+1))

		for it.Seek(startKey); it.ValidForPrefix([]byte(walSequencePrefix)); it.Next() {
			if len(entries) >= limit {
				break
			}

			// Check context
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Get entry ID from sequence mapping
			var entryID string
			err := it.Item().Value(func(val []byte) error {
				entryID = string(val)
				return nil
			})
			if err != nil {
				return err
			}

			// Get the actual entry
			entryKey := []byte(fmt.Sprintf("%s%s", walEntryPrefix, entryID))
			item, err := txn.Get(entryKey)
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					// Entry was deleted, skip
					continue
				}
				return err
			}

			var entry WALEntry
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				return err
			}

			// Verify checksum
			if !entry.VerifyChecksum() {
				w.logger.Warn("WAL entry checksum mismatch",
					zap.String("id", entry.ID),
					zap.Uint64("sequence", entry.Sequence),
				)
				continue
			}

			entries = append(entries, &entry)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read WAL entries: %w", err)
	}

	return entries, nil
}

// ReadByID returns entries after the given entry ID.
func (w *BadgerWAL) ReadByID(ctx context.Context, afterID string, limit int) ([]*WALEntry, error) {
	if w.closed.Load() {
		return nil, ErrWALClosed
	}

	// If no afterID, start from beginning
	if afterID == "" {
		return w.Read(ctx, 0, limit)
	}

	// Get the sequence number for the afterID
	entry, err := w.GetEntry(ctx, afterID)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			// Entry not found, start from beginning
			return w.Read(ctx, 0, limit)
		}
		return nil, err
	}

	return w.Read(ctx, entry.Sequence, limit)
}

// GetEntry returns a specific entry by ID.
func (w *BadgerWAL) GetEntry(ctx context.Context, id string) (*WALEntry, error) {
	if w.closed.Load() {
		return nil, ErrWALClosed
	}

	var entry WALEntry

	err := w.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("%s%s", walEntryPrefix, id))
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &entry)
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, fmt.Errorf("WAL entry not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get WAL entry: %w", err)
	}

	// Verify checksum
	if !entry.VerifyChecksum() {
		return nil, ErrChecksumMismatch
	}

	return &entry, nil
}

// Position returns the current WAL position.
func (w *BadgerWAL) Position(ctx context.Context) (*WALPosition, error) {
	if w.closed.Load() {
		return nil, ErrWALClosed
	}

	currentSeq := w.sequence.Load()
	if currentSeq == 0 {
		return &WALPosition{
			Sequence:  0,
			Timestamp: time.Now().UTC(),
		}, nil
	}

	// Find the entry at the current sequence
	var pos WALPosition

	err := w.db.View(func(txn *badger.Txn) error {
		seqKey := []byte(fmt.Sprintf("%s%020d", walSequencePrefix, currentSeq))
		item, err := txn.Get(seqKey)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				// No entry at this sequence, return current position
				pos = WALPosition{
					Sequence:  currentSeq,
					Timestamp: time.Now().UTC(),
				}
				return nil
			}
			return err
		}

		var entryID string
		err = item.Value(func(val []byte) error {
			entryID = string(val)
			return nil
		})
		if err != nil {
			return err
		}

		// Get entry timestamp
		entryKey := []byte(fmt.Sprintf("%s%s", walEntryPrefix, entryID))
		item, err = txn.Get(entryKey)
		if err != nil {
			return err
		}

		var entry WALEntry
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &entry)
		})
		if err != nil {
			return err
		}

		pos = WALPosition{
			Sequence:  currentSeq,
			EntryID:   entryID,
			Timestamp: entry.Timestamp,
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get WAL position: %w", err)
	}

	return &pos, nil
}

// Truncate removes entries before the given sequence number.
func (w *BadgerWAL) Truncate(ctx context.Context, beforeSequence uint64) error {
	if w.closed.Load() {
		return ErrWALClosed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var keysToDelete [][]byte

	// Collect keys to delete
	err := w.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		// Find sequence keys to delete
		for it.Seek([]byte(walSequencePrefix)); it.ValidForPrefix([]byte(walSequencePrefix)); it.Next() {
			key := it.Item().KeyCopy(nil)

			// Parse sequence number from key
			var seq uint64
			_, err := fmt.Sscanf(string(key), walSequencePrefix+"%d", &seq)
			if err != nil {
				continue
			}

			if seq >= beforeSequence {
				break
			}

			keysToDelete = append(keysToDelete, key)

			// Get entry ID to delete the entry too
			var entryID string
			err = it.Item().Value(func(val []byte) error {
				entryID = string(val)
				return nil
			})
			if err == nil && entryID != "" {
				entryKey := []byte(fmt.Sprintf("%s%s", walEntryPrefix, entryID))
				keysToDelete = append(keysToDelete, entryKey)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to collect keys for truncation: %w", err)
	}

	if len(keysToDelete) == 0 {
		return nil
	}

	// Delete in batches
	wb := w.db.NewWriteBatch()
	defer wb.Cancel()

	for _, key := range keysToDelete {
		if err := wb.Delete(key); err != nil {
			return fmt.Errorf("failed to delete key during truncation: %w", err)
		}
	}

	if err := wb.Flush(); err != nil {
		return fmt.Errorf("failed to flush truncation batch: %w", err)
	}

	w.logger.Info("truncated WAL",
		zap.Uint64("before_sequence", beforeSequence),
		zap.Int("entries_deleted", len(keysToDelete)/2),
	)

	return nil
}

// Sync ensures all entries are persisted to disk.
func (w *BadgerWAL) Sync(ctx context.Context) error {
	if w.closed.Load() {
		return ErrWALClosed
	}

	return w.db.Sync()
}

// Close closes the WAL.
func (w *BadgerWAL) Close() error {
	if w.closed.Swap(true) {
		return nil // Already closed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	return w.db.Close()
}

// Stats returns statistics about the WAL.
func (w *BadgerWAL) Stats(ctx context.Context) (*WALStats, error) {
	if w.closed.Load() {
		return nil, ErrWALClosed
	}

	var entryCount int64
	var totalBytes int64
	var oldestSequence uint64
	var oldestTime time.Time
	var newestTime time.Time

	err := w.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		first := true
		for it.Seek([]byte(walEntryPrefix)); it.ValidForPrefix([]byte(walEntryPrefix)); it.Next() {
			entryCount++
			totalBytes += it.Item().EstimatedSize()

			// Get timestamps from first and last entries
			var entry WALEntry
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				continue
			}

			if first {
				oldestSequence = entry.Sequence
				oldestTime = entry.Timestamp
				first = false
			}
			newestTime = entry.Timestamp
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to compute WAL stats: %w", err)
	}

	return &WALStats{
		EntryCount:     entryCount,
		TotalBytes:     totalBytes,
		CurrentSeq:     w.sequence.Load(),
		OldestSequence: oldestSequence,
		OldestTime:     oldestTime,
		NewestTime:     newestTime,
	}, nil
}

// WALStats provides statistics about the WAL.
type WALStats struct {
	EntryCount     int64     `json:"entry_count"`
	TotalBytes     int64     `json:"total_bytes"`
	CurrentSeq     uint64    `json:"current_sequence"`
	OldestSequence uint64    `json:"oldest_sequence"`
	OldestTime     time.Time `json:"oldest_time"`
	NewestTime     time.Time `json:"newest_time"`
}
