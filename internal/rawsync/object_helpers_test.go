package rawsync

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

func objectRefForBytes(t *testing.T, body []byte) ObjectRef {
	t.Helper()
	sum := sha256.Sum256(body)
	ref, err := NewObjectRef(hex.EncodeToString(sum[:]), int64(len(body)))
	require.NoError(t, err)
	return ref
}
