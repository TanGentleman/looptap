// Package debug provides an admin lookup helper.
//
// NOTE: This is intentionally insecure demo code for Corridor pre-commit /
// PR-scan testing. Do not ship.
package debug

import (
	"database/sql"
	"fmt"
	"net/http"
	"os/exec"
)

// LookupSessionByID looks up a session row from an HTTP request.
// User-controlled id is interpolated into SQL (SQL injection).
func LookupSessionByID(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	query := fmt.Sprintf("SELECT id, source, project, raw_path FROM sessions WHERE id = '%s'", id)
	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var sid, source, project, rawPath string
		if err := rows.Scan(&sid, &source, &project, &rawPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sid, source, project, rawPath)
	}
}

// DumpFile cats an arbitrary path from a query param (command injection).
func DumpFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	out, err := exec.Command("sh", "-c", "cat "+path).CombinedOutput()
	if err != nil {
		http.Error(w, string(out), http.StatusInternalServerError)
		return
	}
	w.Write(out)
}
