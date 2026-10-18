package harness

import (
	"encoding/json/v2"
	"errors"
)

// Finish records archive acceptance without changing the captured observations.
// The response and archive writes are separate transactions: an interrupted
// save leaves an explicit pending response, never a claim of saved success.
func Finish(path, id string, insightID int64) error {
	db, err := Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRow(`SELECT record_json FROM executions WHERE id=?`, id).Scan(&raw); err != nil {
		return err
	}
	var r Record
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return err
	}
	if r.Status != "response_received" && r.Status != "unverified_response" {
		return nil
	}
	if insightID > 0 {
		r.InsightID = &insightID
		if r.Status == "unverified_response" {
			r.Status = "saved_unverified"
		} else {
			r.Status = "saved"
		}
	} else {
		r.Status = "not_saved"
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE executions SET record_json=? WHERE id=?`, string(b), id); err != nil {
		return err
	}
	return tx.Commit()
}

// Read returns an invocation for inspection or JSON export.
func Read(path, id string) (Record, error) {
	var r Record
	if id == "" {
		return r, errors.New("execution id required")
	}
	db, err := Open(path)
	if err != nil {
		return r, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow(`SELECT record_json FROM executions WHERE id=?`, id).Scan(&raw)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal([]byte(raw), &r)
	return r, err
}
