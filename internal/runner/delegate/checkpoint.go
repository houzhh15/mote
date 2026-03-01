package delegate

import (
	"encoding/json"
	"fmt"

	"mote/internal/runner/delegate/cfg"
	"mote/internal/storage"
)

const pdaCheckpointKey = "pda_checkpoint"
const pdaSessionKey = "pda_session"

// SavePDACheckpoint persists a PDA checkpoint into Session.Metadata
// using a read-modify-write pattern to preserve other metadata keys.
func SavePDACheckpoint(store *storage.DB, sessionID string, cp *cfg.PDACheckpoint) error {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}

	meta, err := readMetadataMap(sess.Metadata)
	if err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}

	cpJSON, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	meta[pdaCheckpointKey] = json.RawMessage(cpJSON)

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	return store.UpdateSession(sessionID, json.RawMessage(raw))
}

// LoadPDACheckpoint reads a PDA checkpoint from Session.Metadata.
// Returns (nil, nil) if no checkpoint is stored.
func LoadPDACheckpoint(store *storage.DB, sessionID string) (*cfg.PDACheckpoint, error) {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", sessionID, err)
	}

	meta, err := readMetadataMap(sess.Metadata)
	if err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	cpRaw, ok := meta[pdaCheckpointKey]
	if !ok {
		return nil, nil // no checkpoint stored
	}

	var cp cfg.PDACheckpoint
	if err := json.Unmarshal(cpRaw, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

// ClearPDACheckpoint removes the PDA checkpoint from Session.Metadata.
func ClearPDACheckpoint(store *storage.DB, sessionID string) error {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}

	meta, err := readMetadataMap(sess.Metadata)
	if err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}

	if _, ok := meta[pdaCheckpointKey]; !ok {
		return nil // nothing to clear
	}

	delete(meta, pdaCheckpointKey)

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	return store.UpdateSession(sessionID, json.RawMessage(raw))
}

// readMetadataMap decodes Session.Metadata into a mutable map.
// Returns an empty map if metadata is nil or empty.
func readMetadataMap(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return make(map[string]json.RawMessage), nil
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	if meta == nil {
		meta = make(map[string]json.RawMessage)
	}
	return meta, nil
}

// MarkPDASession sets a permanent "pda_session" flag in session metadata.
// Unlike pda_checkpoint (which is transient), this flag persists after
// completion and allows the UI to always recognize the session as PDA.
func MarkPDASession(store *storage.DB, sessionID string, agentName string) error {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}

	meta, err := readMetadataMap(sess.Metadata)
	if err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}

	// Already marked — skip write
	if _, ok := meta[pdaSessionKey]; ok {
		return nil
	}

	info := map[string]string{"agent": agentName}
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal pda_session info: %w", err)
	}
	meta[pdaSessionKey] = json.RawMessage(infoJSON)

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	return store.UpdateSession(sessionID, json.RawMessage(raw))
}
