package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"go.kenn.io/agentsview/internal/db"
)

// registerSessionAnnotationRoutes owns caller-supplied session metadata:
// labels and launcher-supplied parent links. Both are stored apart from
// parsed transcript data and accept session IDs that have not synced yet,
// so a launcher can record them as soon as it starts a session.
func (s *Server) registerSessionAnnotationRoutes() {
	group := huma.NewGroup(s.api, "/api/v1")
	configureRouteGroup(group, "Sessions")

	s.get(group, "/sessions/{id}/summary", "Read session summary", s.humaGetSessionSummary)
	s.put(group, "/sessions/{id}/summary", "Save session summary", s.humaSetSessionSummary)
	s.patch(group, "/sessions/{id}/labels", "Change session labels", s.humaUpdateSessionLabels)
	s.put(group, "/sessions/{id}/parent", "Set or remove session parent link", s.humaSetSessionParent)
}

type updateSessionLabelsInput struct {
	ID   string `path:"id" required:"true" doc:"Session ID"`
	Body struct {
		Clear  bool     `json:"clear,omitempty" doc:"Remove every existing label before adding; with add, replaces the set"`
		Add    []string `json:"add,omitempty" doc:"Labels to add. Labels are free text, often key=value."`
		Remove []string `json:"remove,omitempty" doc:"Labels to remove; absent labels are ignored"`
	}
}

type setSessionParentInput struct {
	ID   string `path:"id" required:"true" doc:"Session ID"`
	Body struct {
		ParentSessionID string `json:"parent_session_id,omitempty" doc:"ID of the session that launched this one; empty or omitted removes the link"`
	}
}

func (s *Server) humaUpdateSessionLabels(
	ctx context.Context, in *updateSessionLabelsInput,
) (*jsonOutput[db.SessionLabels], error) {
	localDB, _, err := s.localWorktreeMappingHumaDB()
	if err != nil {
		return nil, err
	}
	engine := s.syncEngineForLocal(ctx, localDB)
	var labels db.SessionLabels
	if in.Body.Clear {
		labels, err = engine.SetSessionLabels(ctx, in.ID, in.Body.Add)
	} else {
		labels, err = engine.UpdateSessionLabels(ctx, in.ID, in.Body.Add, in.Body.Remove)
	}
	if err != nil {
		return nil, sessionAnnotationError("update session labels", err)
	}
	return &jsonOutput[db.SessionLabels]{Body: labels}, nil
}

func (s *Server) humaSetSessionParent(
	ctx context.Context, in *setSessionParentInput,
) (*jsonOutput[db.SessionExternalParent], error) {
	localDB, _, err := s.localWorktreeMappingHumaDB()
	if err != nil {
		return nil, err
	}
	engine := s.syncEngineForLocal(ctx, localDB)
	var link db.SessionExternalParent
	if strings.TrimSpace(in.Body.ParentSessionID) == "" {
		link, err = engine.ClearSessionExternalParent(ctx, in.ID)
	} else {
		link, err = engine.SetSessionExternalParent(ctx, in.ID, in.Body.ParentSessionID)
	}
	if err != nil {
		return nil, sessionAnnotationError("set session parent", err)
	}
	return &jsonOutput[db.SessionExternalParent]{Body: link}, nil
}

func sessionAnnotationError(op string, err error) error {
	switch {
	case errors.Is(err, db.ErrSessionLabelsInvalid),
		errors.Is(err, db.ErrSessionExternalParentInvalid):
		return apiError(http.StatusBadRequest, err.Error())
	}
	if handled := handleHumaReadOnly(err); handled != nil {
		return handled
	}
	return internalError(op, err)
}

type sessionSummaryInput struct {
	ID string `path:"id" required:"true"`
}
type setSessionSummaryInput struct {
	ID   string `path:"id" required:"true"`
	Body struct {
		Content string `json:"content" maxLength:"16000"`
	}
}

func (s *Server) humaGetSessionSummary(ctx context.Context, in *sessionSummaryInput) (*jsonOutput[db.SessionSummary], error) {
	local, ok := s.db.(*db.DB)
	if !ok {
		return nil, apiError(http.StatusNotImplemented, "local archive required")
	}
	result, err := local.GetSessionSummary(ctx, in.ID)
	if err != nil {
		return nil, internalError("read summary", err)
	}
	return &jsonOutput[db.SessionSummary]{Body: result}, nil
}
func (s *Server) humaSetSessionSummary(ctx context.Context, in *setSessionSummaryInput) (*jsonOutput[db.SessionSummary], error) {
	local, _, err := s.localWorktreeMappingHumaDB()
	if err != nil {
		return nil, err
	}
	result, err := s.syncEngineForLocal(ctx, local).SetSessionSummary(ctx, in.ID, in.Body.Content)
	if errors.Is(err, db.ErrSessionSummaryInvalid) {
		return nil, apiError(http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return nil, sessionAnnotationError("save summary", err)
	}
	return &jsonOutput[db.SessionSummary]{Body: result}, nil
}
