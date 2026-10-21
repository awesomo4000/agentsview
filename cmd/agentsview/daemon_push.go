package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"

	"go.kenn.io/agentsview/internal/apiclient"
)

// daemonPushOperation invokes a streaming local daemon operation.
type daemonPushOperation func(
	ctx context.Context, api *apiclient.Client,
) (*http.Response, []byte, *runtime.Stream[[]byte], error)

func startupSyncOperation(
	ctx context.Context, api *apiclient.Client,
) (*http.Response, []byte, *runtime.Stream[[]byte], error) {
	response, err := api.PostAPIV1SyncStreamWithResponse(
		ctx, &apiclient.PostAPIV1SyncRequestOptions{
			Query: &apiclient.PostAPIV1SyncQuery{Wait: new(true), StartupOnly: new(true)},
		},
	)
	if response == nil {
		return nil, nil, nil, err
	}
	return response.HTTPResponse, response.Body, response.Stream200, nil
}

// postDaemonPush delegates a push to the local daemon. It negotiates an SSE
// response so the daemon can stream per-phase progress while the push runs;
// each progress event is decoded as P and handed to onProgress (which may be
// nil). A plain JSON response — the daemon streams only when it can flush —
// is decoded directly as the result T.
func postDaemonPush[T, P any](
	ctx context.Context,
	tr transport,
	authToken string,
	operation daemonPushOperation,
	onProgress func(P),
) (T, error) {
	var zero T
	for {
		api, err := apiclient.NewHTTPClient(tr.URL, authToken, &http.Client{Timeout: 0})
		if err != nil {
			return zero, err
		}
		resp, payload, stream, err := operation(ctx, api)
		if resp == nil {
			return zero, err
		}
		if resp.StatusCode != http.StatusOK {
			msg := payload
			_ = resp.Body.Close()

			return zero, errors.New(daemonErrorMessage(resp.StatusCode, msg))
		}
		defer resp.Body.Close()
		if strings.HasPrefix(
			resp.Header.Get("Content-Type"), "text/event-stream",
		) {
			return consumeDaemonPushEvents[T](stream, onProgress)
		}
		var out T
		if err := json.Unmarshal(payload, &out); err != nil {
			return zero, err
		}
		return out, nil
	}
}

// consumeDaemonPushEvents applies daemon progress and terminal events decoded
// by the generated client's stream.
func consumeDaemonPushEvents[T, P any](stream *runtime.Stream[[]byte], onProgress func(P)) (T, error) {
	var result, zero T
	var done bool
	var pushErr error
	defer stream.Close()
	for stream.Next() {
		frame := stream.Event()
		if len(frame.Data) == 0 {
			continue
		}
		switch frame.Type {
		case "done", "report":
			if err := json.Unmarshal(frame.Data, &result); err != nil {
				return zero, fmt.Errorf("decoding daemon push result: %w", err)
			}
			done = true
		case "progress":
			if onProgress != nil {
				var progress P
				if err := json.Unmarshal(frame.Data, &progress); err != nil {
					return zero, fmt.Errorf("decoding daemon push progress: %w", err)
				}
				onProgress(progress)
			}
		default:
			var apiErr apiclient.APIErrorResponse
			if err := json.Unmarshal(frame.Data, &apiErr); err == nil && apiErr.ErrorData != "" {
				pushErr = errors.New(apiErr.ErrorData)
			} else {
				pushErr = fmt.Errorf("daemon push error: %s", frame.Data)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return zero, err
	}
	if pushErr != nil {
		return zero, pushErr
	}
	if !done {
		return zero, errors.New("daemon push response missing done event")
	}
	return result, nil
}
