package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/server/handlers"
)

const (
	deadTasksDefaultLimit = 100
	deadTasksMaxLimit     = 500
)

var errDeadTasksOff = errors.New("failed background tasks are only kept when clustering is on")

type deadTasksPage struct {
	Items      []cluster.DeadTask `json:"items"`
	NextBefore *uint64            `json:"next_before"`
}

func (a *API) DeadTasks(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	if nil == a.Deps.DeadTasks {
		handlers.SendJSONResponse(w, deadTasksPage{Items: []cluster.DeadTask{}})
		return
	}
	query := r.URL.Query()
	limit := deadTasksDefaultLimit
	if raw := query.Get("limit"); "" != raw {
		parsed, err := strconv.Atoi(raw)
		if nil != err || 1 > parsed || deadTasksMaxLimit < parsed {
			handlers.SendErrorResponse(w, logger, fmt.Errorf("limit must be between 1 and %d", deadTasksMaxLimit), http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	var before uint64
	if raw := query.Get("before"); "" != raw {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if nil != err {
			handlers.SendErrorResponse(w, logger, errors.New("before must be a task id"), http.StatusBadRequest)
			return
		}
		before = parsed
	}
	items, more, err := a.Deps.DeadTasks.List(r.Context(), limit, before)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	page := deadTasksPage{Items: items}
	if more && 0 < len(items) {
		next := items[len(items)-1].ID
		page.NextBefore = &next
	}
	handlers.SendJSONResponse(w, page)
}

func (a *API) RetryDeadTask(w http.ResponseWriter, r *http.Request) {
	a.mutateDeadTask(w, r, "retry", func(id uint64) error { return a.Deps.DeadTasks.Retry(r.Context(), id) })
}

func (a *API) DropDeadTask(w http.ResponseWriter, r *http.Request) {
	a.mutateDeadTask(w, r, "drop", func(id uint64) error { return a.Deps.DeadTasks.Drop(r.Context(), id) })
}

func (a *API) mutateDeadTask(w http.ResponseWriter, r *http.Request, action string, run func(id uint64) error) {
	logger := zap.L()
	if nil == a.Deps.DeadTasks {
		handlers.SendErrorResponse(w, logger, errDeadTasksOff, http.StatusConflict)
		return
	}
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if nil != err || 0 == id {
		handlers.SendErrorResponse(w, logger, errors.New("id must be a task id"), http.StatusBadRequest)
		return
	}
	if err := run(id); nil != err {
		status := http.StatusInternalServerError
		if errors.Is(err, cluster.ErrDeadTaskNotFound) {
			status = http.StatusNotFound
		}
		handlers.SendErrorResponse(w, logger, err, status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	logger.Info("Failed background task "+action, zap.Uint64("task_id", id))
}
