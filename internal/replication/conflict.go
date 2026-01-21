package replication

import (
	"context"
	"encoding/json"
)

// LastWriteWinsResolver resolves conflicts by using the most recent timestamp.
type LastWriteWinsResolver struct{}

// NewLastWriteWinsResolver creates a new last-write-wins conflict resolver.
func NewLastWriteWinsResolver() *LastWriteWinsResolver {
	return &LastWriteWinsResolver{}
}

// Resolve returns the entry with the latest timestamp.
func (r *LastWriteWinsResolver) Resolve(ctx context.Context, local, remote *WALEntry) (*WALEntry, error) {
	if remote.Timestamp.After(local.Timestamp) {
		return remote, nil
	}
	return local, nil
}

// MergeResolver attempts to merge non-conflicting changes.
type MergeResolver struct {
	// Fallback resolver when merge is not possible
	fallback ConflictResolver
}

// NewMergeResolver creates a new merge conflict resolver.
func NewMergeResolver() *MergeResolver {
	return &MergeResolver{
		fallback: NewLastWriteWinsResolver(),
	}
}

// Resolve attempts to merge changes, falling back to last-write-wins.
func (r *MergeResolver) Resolve(ctx context.Context, local, remote *WALEntry) (*WALEntry, error) {
	// Only attempt merge for updates to the same resource
	if local.Operation != OperationUpdate || remote.Operation != OperationUpdate {
		return r.fallback.Resolve(ctx, local, remote)
	}

	// Can only merge memory updates
	if local.ResourceType != ResourceTypeMemory || remote.ResourceType != ResourceTypeMemory {
		return r.fallback.Resolve(ctx, local, remote)
	}

	// Try to merge memory metadata
	merged, err := r.mergeMemoryData(local.Data, remote.Data)
	if err != nil {
		return r.fallback.Resolve(ctx, local, remote)
	}

	// Use the later timestamp for the merged entry
	winner := local
	if remote.Timestamp.After(local.Timestamp) {
		winner = remote
	}

	// Create a new entry with merged data
	result := &WALEntry{
		ID:           winner.ID,
		Sequence:     winner.Sequence,
		Timestamp:    winner.Timestamp,
		TenantID:     winner.TenantID,
		Operation:    winner.Operation,
		ResourceType: winner.ResourceType,
		ResourceID:   winner.ResourceID,
		Namespace:    winner.Namespace,
		Data:         merged,
		Region:       winner.Region,
		Replicated:   true,
	}
	result.Checksum = result.ComputeChecksum()

	return result, nil
}

// mergeMemoryData attempts to merge two memory JSON objects.
func (r *MergeResolver) mergeMemoryData(localData, remoteData []byte) ([]byte, error) {
	var local, remote map[string]interface{}

	if err := json.Unmarshal(localData, &local); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(remoteData, &remote); err != nil {
		return nil, err
	}

	// Merge strategy:
	// - Use remote content if different (content wins from remote)
	// - Merge metadata maps
	// - Use higher access count
	// - Use later timestamp for updated_at

	merged := make(map[string]interface{})
	for k, v := range local {
		merged[k] = v
	}

	// Merge specific fields
	if remoteContent, ok := remote["content"]; ok {
		merged["content"] = remoteContent
	}

	// Merge metadata
	if localMeta, ok := local["metadata"].(map[string]interface{}); ok {
		if remoteMeta, ok := remote["metadata"].(map[string]interface{}); ok {
			mergedMeta := make(map[string]interface{})
			for k, v := range localMeta {
				mergedMeta[k] = v
			}
			for k, v := range remoteMeta {
				mergedMeta[k] = v
			}
			merged["metadata"] = mergedMeta
		}
	}

	// Merge tags (union)
	if localTags, ok := local["tags"].([]interface{}); ok {
		if remoteTags, ok := remote["tags"].([]interface{}); ok {
			tagSet := make(map[string]bool)
			for _, t := range localTags {
				if s, ok := t.(string); ok {
					tagSet[s] = true
				}
			}
			for _, t := range remoteTags {
				if s, ok := t.(string); ok {
					tagSet[s] = true
				}
			}
			mergedTags := make([]string, 0, len(tagSet))
			for t := range tagSet {
				mergedTags = append(mergedTags, t)
			}
			merged["tags"] = mergedTags
		}
	}

	// Use higher access count
	localCount, _ := local["access_count"].(float64)
	remoteCount, _ := remote["access_count"].(float64)
	if remoteCount > localCount {
		merged["access_count"] = remoteCount
	}

	return json.Marshal(merged)
}

// RejectResolver rejects all conflicts.
type RejectResolver struct{}

// NewRejectResolver creates a new reject conflict resolver.
func NewRejectResolver() *RejectResolver {
	return &RejectResolver{}
}

// Resolve always returns an error for conflicts.
func (r *RejectResolver) Resolve(ctx context.Context, local, remote *WALEntry) (*WALEntry, error) {
	return nil, ErrConflict
}

// NewConflictResolver creates a conflict resolver based on strategy.
func NewConflictResolver(strategy ConflictStrategy) ConflictResolver {
	switch strategy {
	case ConflictMerge:
		return NewMergeResolver()
	case ConflictReject:
		return NewRejectResolver()
	default:
		return NewLastWriteWinsResolver()
	}
}
