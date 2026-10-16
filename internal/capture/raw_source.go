package capture

import (
	"errors"
	"fmt"
	"strings"
)

var errInvalidRawSource = errors.New("invalid raw source")

// RawSourceRef identifies one bounded raw provider artifact without embedding
// its contents in a portable manifest.
type RawSourceRef struct {
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type,omitempty"`
	Path      string `json:"path,omitempty"`
}

// rawSourceMediaTypes is the fixed allowlist for raw source snapshots.
// application/jsonl covers JSONL agent session files (claude, codex).
var rawSourceMediaTypes = map[string]bool{
	"application/jsonl": true,
}

// maxRawSourceSize bounds one raw source snapshot (1 GiB).
const maxRawSourceSize = int64(1 << 30)

// validateRawSource checks a manifest's optional raw_source reference
// against the stable wire contract. A nil reference is valid (raw capture is
// optional).
func validateRawSource(raw *RawSourceRef) error {
	if raw == nil {
		return nil
	}
	if err := validateRawSourceHash(raw.Hash); err != nil {
		return err
	}
	if raw.Size < 0 || raw.Size > maxRawSourceSize {
		return fmt.Errorf("%w: raw source size out of range", errInvalidRawSource)
	}
	if raw.MediaType != "" && !rawSourceMediaTypes[raw.MediaType] {
		return fmt.Errorf("%w: unsupported raw source media type %q",
			errInvalidRawSource, raw.MediaType)
	}
	if raw.Path != "" {
		if strings.Contains(raw.Path, "\\") || strings.Contains(raw.Path, "..") {
			return fmt.Errorf("%w: invalid raw source path", errInvalidRawSource)
		}
		// A colon anywhere rejects Windows drive-qualified (C:/...) and
		// volume-relative (C:...) paths, NTFS alternate data streams, and
		// URI schemes in one platform-neutral rule.
		if strings.HasPrefix(raw.Path, "/") || strings.Contains(raw.Path, ":") {
			return fmt.Errorf("%w: raw source path must be relative",
				errInvalidRawSource)
		}
	}
	return nil
}

func validateRawSourceHash(hash string) error {
	if len(hash) != 64 {
		return fmt.Errorf("%w: invalid artifact hash", errInvalidRawSource)
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fmt.Errorf("%w: invalid artifact hash", errInvalidRawSource)
		}
	}
	return nil
}
