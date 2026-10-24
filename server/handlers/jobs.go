package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/server/middleware"
	"github.com/ebogdum/callfs/telemetry"
)

const (
	jobsDefaultLimit = 50
	jobsMaxLimit     = 200
	eventsMaxLimit   = telemetry.MaxPageLimit
)

type jobsListResponse struct {
	Items      []jobs.Job `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type eventsResponse struct {
	Items []telemetry.Event `json:"items"`
}

func V1ListMyJobs(runner *jobs.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		query := r.URL.Query()
		states, err := ParseJobStates(query.Get("state"))
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		limit, err := parseJobsLimit(query.Get("limit"))
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		page, more, err := runner.ListPage(jobs.Filter{CreatedBy: userID, States: states}, query.Get("cursor"), limit)
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		response := jobsListResponse{Items: page}
		if more && 0 < len(page) {
			next := page[len(page)-1].ID
			response.NextCursor = &next
		}
		SendJSONResponse(w, response)
	}
}

func V1GetJob(runner *jobs.Runner, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		job, err := loadVisibleJob(runner, admins, userID, chi.URLParam(r, "id"))
		if nil != err {
			sendJobError(w, logger, err)
			return
		}
		SendJSONResponse(w, job)
	}
}

func V1CancelJob(runner *jobs.Runner, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if _, err := loadVisibleJob(runner, admins, userID, id); nil != err {
			sendJobError(w, logger, err)
			return
		}
		job, err := runner.Cancel(id)
		if nil != err {
			sendJobError(w, logger, err)
			return
		}
		SendJSONResponse(w, job)
		logger.Info("Job cancel requested", zap.String("job_id", id), zap.String("user_id", userID))
	}
}

func V1RetryJobFailed(runner *jobs.Runner, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if _, err := loadVisibleJob(runner, admins, userID, id); nil != err {
			sendJobError(w, logger, err)
			return
		}
		job, err := runner.RetryFailed(id, userID)
		if nil != err {
			sendJobError(w, logger, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(job); nil != err {
			logger.Error("Failed to encode job response", zap.Error(err))
		}
		logger.Info("Failed items retry started", zap.String("job_id", job.ID), zap.String("retry_of", id), zap.String("user_id", userID))
	}
}

func V1MyEvents(hub *telemetry.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		since, err := parseEventsSince(r.URL.Query().Get("since"))
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		limit, err := parseEventsLimit(r.URL.Query().Get("limit"))
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		events, err := hub.Events.Query(telemetry.EventQuery{Actor: userID, Since: since, Limit: limit})
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		SendJSONResponse(w, eventsResponse{Items: events})
	}
}

func ParseJobStates(raw string) ([]jobs.State, error) {
	if "" == raw {
		return nil, nil
	}
	seen := make(map[jobs.State]bool)
	var states []jobs.State
	for _, part := range strings.Split(raw, ",") {
		state := jobs.State(strings.TrimSpace(part))
		if !validJobState(state) {
			return nil, errors.New("state must be one of queued, running, cancelling, succeeded, partial, failed, cancelled")
		}
		if !seen[state] {
			seen[state] = true
			states = append(states, state)
		}
	}
	return states, nil
}

func validJobState(state jobs.State) bool {
	switch state {
	case jobs.StateQueued, jobs.StateRunning, jobs.StateCancelling, jobs.StateSucceeded, jobs.StatePartial, jobs.StateFailed, jobs.StateCancelled:
		return true
	}
	return false
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

func jobVisibleTo(job jobs.Job, userID string, admins *auth.AdminSet) bool {
	return userID == job.CreatedBy || admins.IsAdmin(userID)
}

func loadVisibleJob(runner *jobs.Runner, admins *auth.AdminSet, userID, id string) (jobs.Job, error) {
	job, err := runner.Get(id)
	if nil != err {
		return jobs.Job{}, err
	}
	if !jobVisibleTo(job, userID, admins) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return job, nil
}

func sendJobError(w http.ResponseWriter, logger *zap.Logger, err error) {
	SendErrorResponse(w, logger, err, jobErrorStatus(err))
}

func jobErrorStatus(err error) int {
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, jobs.ErrInvalidState), errors.Is(err, jobs.ErrNothingToRetry):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func parseEventsSince(raw string) (time.Time, error) {
	if "" == raw {
		return time.Time{}, nil
	}
	since, err := time.Parse(time.RFC3339Nano, raw)
	if nil != err {
		return time.Time{}, errors.New("since must be an RFC 3339 time")
	}
	return since, nil
}

func parseEventsLimit(raw string) (int, error) {
	if "" == raw {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err || 1 > limit || eventsMaxLimit < limit {
		return 0, fmt.Errorf("limit must be between 1 and %d", eventsMaxLimit)
	}
	return limit, nil
}
