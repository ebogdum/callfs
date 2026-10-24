package admin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	jobsDefaultLimit    = 200
	jobsMaxLimit        = 1000
	jobsMaxFilterLength = 128
)

type jobsPage struct {
	Items      []jobs.Job `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type workersPage struct {
	Items []jobs.WorkerStatus `json:"items"`
}

type jobAccepted struct {
	JobID string `json:"job_id"`
}

func (a *API) Jobs(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	query := r.URL.Query()
	states, err := handlers.ParseJobStates(query.Get("state"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	kind := query.Get("kind")
	createdBy := query.Get("created_by")
	if jobsMaxFilterLength < len(kind) || jobsMaxFilterLength < len(createdBy) {
		handlers.SendErrorResponse(w, logger, errors.New("kind and created_by must be at most 128 characters"), http.StatusBadRequest)
		return
	}
	limit, err := parseJobsLimit(query.Get("limit"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	cursorID, err := jobsCursorID(query.Get("cursor"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	items, more, err := a.Deps.Jobs.ListPage(jobs.Filter{Kind: kind, CreatedBy: createdBy, States: states}, cursorID, limit)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	page := jobsPage{Items: items}
	if more && 0 < len(page.Items) {
		next := base64.RawURLEncoding.EncodeToString([]byte(page.Items[len(page.Items)-1].ID))
		page.NextCursor = &next
	}
	handlers.SendJSONResponse(w, page)
}

func (a *API) Job(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	job, err := a.Deps.Jobs.Get(chi.URLParam(r, "id"))
	if nil != err {
		sendJobError(w, logger, err)
		return
	}
	handlers.SendJSONResponse(w, job)
}

func (a *API) CancelJob(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	job, err := a.Deps.Jobs.Cancel(id)
	if nil != err {
		sendJobError(w, logger, err)
		return
	}
	handlers.SendJSONResponse(w, job)
	logger.Info("Job cancel requested", zap.String("job_id", id), zap.String("user_id", actor))
}

func (a *API) RetryJobFailed(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	job, err := a.Deps.Jobs.RetryFailed(id, actor)
	if nil != err {
		sendJobError(w, logger, err)
		return
	}
	sendAccepted(w, logger, job)
	logger.Info("Failed items retry started", zap.String("job_id", job.ID), zap.String("retry_of", id), zap.String("user_id", actor))
}

func (a *API) Workers(w http.ResponseWriter, r *http.Request) {
	handlers.SendJSONResponse(w, workersPage{Items: a.Deps.Jobs.Workers()})
}

func (a *API) RunWorker(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	job, err := a.Deps.Jobs.RunNow(name, actor)
	if nil != err {
		sendJobError(w, logger, err)
		return
	}
	sendAccepted(w, logger, jobAccepted{JobID: job.ID})
	logger.Info("Worker run started", zap.String("worker", name), zap.String("job_id", job.ID), zap.String("user_id", actor))
}

func jobActor(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (string, bool) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
		return "", false
	}
	return userID, true
}

func sendAccepted(w http.ResponseWriter, logger *zap.Logger, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(body); nil != err {
		logger.Error("Failed to encode accepted response", zap.Error(err))
	}
}

func sendJobError(w http.ResponseWriter, logger *zap.Logger, err error) {
	handlers.SendErrorResponse(w, logger, err, jobErrorStatus(err))
}

func jobErrorStatus(err error) int {
	switch {
	case errors.Is(err, jobs.ErrNotFound), errors.Is(err, jobs.ErrWorkerNotFound):
		return http.StatusNotFound
	case errors.Is(err, jobs.ErrInvalidState), errors.Is(err, jobs.ErrNothingToRetry), errors.Is(err, jobs.ErrWorkerBusy):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func parseJobsLimit(raw string) (int, error) {
	if "" == raw {
		return jobsDefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err || 1 > limit || jobsMaxLimit < limit {
		return 0, fmt.Errorf("limit must be between 1 and %d", jobsMaxLimit)
	}
	return limit, nil
}

func jobsCursorID(cursor string) (string, error) {
	if "" == cursor {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err {
		return "", errors.New("cursor is not valid")
	}
	return string(decoded), nil
}
