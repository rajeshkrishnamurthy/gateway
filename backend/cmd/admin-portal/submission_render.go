package main

import (
	"log"
	"net/http"
)

func (s *portalServer) renderSubmissionResult(w http.ResponseWriter, status int, view submissionResultView) {
	if s.templates.submissionResult == nil {
		http.Error(w, "template not configured", http.StatusInternalServerError)
		return
	}
	fragment, err := executeTemplate(s.templates.submissionResult, "submission_result.tmpl", view)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if status >= http.StatusBadRequest {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write submission fragment: %v", err)
	}
}

func (s *portalServer) renderSubmissionFailure(w http.ResponseWriter, r *http.Request, status int, message string) {
	if !isHTMX(r) {
		http.Error(w, message, status)
		return
	}
	s.renderSubmissionResult(w, status, submissionResultView{Error: message})
}
