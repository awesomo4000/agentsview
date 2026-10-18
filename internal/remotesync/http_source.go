package remotesync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.kenn.io/agentsview/internal/apiclient"
	"go.kenn.io/agentsview/internal/parser"
)

// HTTPSource fetches remote sync data from a remote agentsview daemon.
type HTTPSource struct {
	URL    string
	Token  string
	Client *http.Client
}

func (s HTTPSource) client() *http.Client {
	return &http.Client{Transport: disabledRemoteTransport{}}
}

// Archive requests an archive and returns its body once the daemon accepts.
func (s HTTPSource) Archive(ctx context.Context, request ArchiveRequest) (*ArchiveStream, error) {
	return nil, fmt.Errorf("remote sync transport has been removed")
}

func positiveContentLength(n int64) int64 {
	if n > 0 {
		return n
	}
	return 0
}

func (s HTTPSource) Manifest(
	ctx context.Context, targets TargetSet,
) (Manifest, bool, error) {
	return Manifest{}, false, fmt.Errorf("remote sync transport has been removed")
}

func (s HTTPSource) Targets(
	ctx context.Context,
) (TargetSet, error) {
	return TargetSet{}, fmt.Errorf("remote sync transport has been removed")
}

func (s HTTPSource) transferHeaders(_ context.Context, req *http.Request) error {
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	SetProtocolHeader(req.Header)
	if req.Method == http.MethodPost {
		req.Header.Set("Accept-Encoding", "zstd, gzip")
	}
	return nil
}

func generatedProviderPaths(paths map[parser.AgentType][]string) map[string][]string {
	result := make(map[string][]string, len(paths))
	for provider, paths := range paths {
		result[string(provider)] = paths
	}
	return result
}

func generatedTargets(targets TargetSet) apiclient.RemotesyncTargetSet {
	return apiclient.RemotesyncTargetSet{
		Dirs: generatedProviderPaths(targets.Dirs), Files: generatedProviderPaths(targets.Files),
		ProviderExtraFiles: generatedProviderPaths(targets.ProviderExtraFiles), ExtraFiles: targets.ExtraFiles,
		CodexIndexFiles: targets.CodexIndexFiles, ForbiddenRoots: targets.ForbiddenRoots,
	}
}

func (s HTTPSource) requestArchive(
	ctx context.Context, request ArchiveRequest,
) (*http.Response, error) {
	return nil, fmt.Errorf("remote sync transport has been removed")
}

// StatusError reports a non-2xx response from a remote daemon's
// remote-sync endpoints. Detail carries the (untrusted) response
// body for local logs; user-facing summaries should rely on Code.
type StatusError struct {
	Code   int
	Status string
	Detail string
}

func (e *StatusError) Error() string {
	msg := e.Detail
	if msg == "" {
		msg = e.Status
	}
	return fmt.Sprintf("remote sync %s: %s", e.Status, msg)
}

func httpStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &StatusError{
		Code:   resp.StatusCode,
		Status: resp.Status,
		Detail: strings.TrimSpace(string(body)),
	}
}

type disabledRemoteTransport struct{}

func (disabledRemoteTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("remote sync transport has been removed")
}
