package admin

import (
	"net/http"

	"github.com/ebogdum/callfs/server/handlers"
)

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	if nil == a.deps.Health {
		a.unavailable(w, "health checker")
		return
	}
	fresh := "true" == r.URL.Query().Get("fresh")
	handlers.SendJSONResponse(w, a.deps.Health.Report(r.Context(), fresh))
}
