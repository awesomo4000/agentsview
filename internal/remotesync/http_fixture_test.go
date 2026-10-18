// Test-only source for archive import, collision, and rebuild coverage.
package remotesync

import (
	"compress/gzip"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"

	"go.kenn.io/agentsview/internal/apiclient"
)

type fixtureHTTPSource struct {
	URL    string
	Token  string
	Client *http.Client
}

func (s fixtureHTTPSource) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	// Archive responses can stream for hours; ctx governs their lifetime.
	return &http.Client{Timeout: 0}
}
func (s fixtureHTTPSource) Archive(ctx context.Context, request ArchiveRequest) (*ArchiveStream, error) {
	resp, err := s.requestArchive(ctx, request)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, httpStatusError(resp)
	}
	if err := ValidateProtocolHeader(resp.Header); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	// The daemon negotiates zstd or gzip from Accept-Encoding. Extraction
	// rejects any other value, so it is passed through unchanged.
	return &ArchiveStream{
		Body:     resp.Body,
		Encoding: ArchiveEncoding(resp.Header.Get("Content-Encoding")),
		Size:     positiveContentLength(resp.ContentLength),
	}, nil
}
func (s fixtureHTTPSource) Manifest(
	ctx context.Context, targets TargetSet,
) (Manifest, bool, error) {
	body := generatedTargets(targets)
	resp, err := apiclient.RawRequest(s.URL, s.client(), func(api *apiclient.Client) error {
		_, err := api.PostAPIV1RemoteSyncManifestWithResponse(ctx, &apiclient.PostAPIV1RemoteSyncManifestRequestOptions{Body: &body})
		return err
	}, s.transferHeaders)
	if err != nil {
		return Manifest{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		if err := ValidateProtocolHeader(resp.Header); err != nil {
			return Manifest{}, false, err
		}
		return Manifest{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Manifest{}, false, httpStatusError(resp)
	}
	if err := ValidateProtocolHeader(resp.Header); err != nil {
		return Manifest{}, false, err
	}
	bodyReader := &httpBodyReader{r: resp.Body}
	reader := io.Reader(bodyReader)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(bodyReader)
		if err != nil {
			if bodyReader.err != nil {
				err = bodyReader.err
			}
			return Manifest{}, false, fmt.Errorf("decode manifest gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	var manifest Manifest
	if err := json.UnmarshalRead(reader, &manifest); err != nil {
		if bodyReader.err != nil {
			err = bodyReader.err
		}
		return Manifest{}, false, fmt.Errorf("decode remote manifest: %w", err)
	}
	return manifest, true, nil
}
func (s fixtureHTTPSource) Targets(
	ctx context.Context,
) (TargetSet, error) {
	resp, err := apiclient.RawRequest(s.URL, s.client(), func(api *apiclient.Client) error {
		_, err := api.GetAPIV1RemoteSyncTargetsWithResponse(ctx, &apiclient.GetAPIV1RemoteSyncTargetsRequestOptions{})
		return err
	}, s.transferHeaders)
	if err != nil {
		return TargetSet{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TargetSet{}, httpStatusError(resp)
	}
	if err := ValidateProtocolHeader(resp.Header); err != nil {
		return TargetSet{}, err
	}
	var targets TargetSet
	bodyReader := &httpBodyReader{r: resp.Body}
	if err := json.UnmarshalRead(bodyReader, &targets); err != nil {
		if bodyReader.err != nil {
			err = bodyReader.err
		}
		return TargetSet{}, fmt.Errorf("decode remote targets: %w", err)
	}
	return targets, nil
}
func (s fixtureHTTPSource) transferHeaders(_ context.Context, req *http.Request) error {
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	SetProtocolHeader(req.Header)
	if req.Method == http.MethodPost {
		req.Header.Set("Accept-Encoding", "zstd, gzip")
	}
	return nil
}
func (s fixtureHTTPSource) requestArchive(
	ctx context.Context, request ArchiveRequest,
) (*http.Response, error) {
	targets := generatedTargets(request.TargetSet)
	body := apiclient.RemotesyncArchiveRequest{
		Dirs: targets.Dirs, Files: targets.Files, ProviderExtraFiles: targets.ProviderExtraFiles,
		ExtraFiles: targets.ExtraFiles, CodexIndexFiles: targets.CodexIndexFiles, ForbiddenRoots: targets.ForbiddenRoots,
	}
	if request.DeltaFiles != nil {
		body.DeltaFiles = new(request.DeltaFiles)
	}
	return apiclient.RawRequest(s.URL, s.client(), func(api *apiclient.Client) error {
		_, err := api.PostAPIV1RemoteSyncArchiveWithResponse(ctx, &apiclient.PostAPIV1RemoteSyncArchiveRequestOptions{Body: &body})
		return err
	}, s.transferHeaders)
}
