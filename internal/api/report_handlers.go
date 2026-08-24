package api

import (
	"net/http"

	"github.com/JCO-Digital/jman/internal/reports"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ListReportsHandler returns metadata for all registered reports.
func ListReportsHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, reports.AllMeta())
}

// RunReportHandler executes a report by ID with the request's query
// parameters and returns its tabular result.
func RunReportHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	report, ok := reports.Get(id)
	if !ok {
		WriteError(w, http.StatusNotFound, "Unknown report")
		return
	}

	result, err := report.Run(r.URL.Query())
	if err != nil {
		verb.LogPrintf(verb.Normal, "RunReportHandler(%s): %v", id, err)
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, result)
}
