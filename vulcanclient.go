/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cenkalti/backoff"
)

const (
	createScanURL        = "%s/v1/teams/%s/scans"
	sendReportURL        = "%s/v1/teams/%s/report/digest"
	bearerHeaderTemplate = "Bearer %s"
)

// ScanRequest contains the payload to send to the API scan endpoint.
type ScanRequest struct {
	ProgramID     string    `json:"program_id"`
	ScheduledTime time.Time `json:"scheduled_time"`
	RequestedBy   string    `json:"requested_by"`
}

// VulcanClient provides functionality for interacting with the vulcan-api.
type VulcanClient struct {
	VulcanAPI   string
	VulcanUser  string
	VulcanToken string
}

// CreateScan creates a scan by calling vulcan-api
func (c *VulcanClient) CreateScan(scanID, teamID string) error {
	scanMsg := ScanRequest{
		ProgramID:     scanID,
		ScheduledTime: time.Now(),
		RequestedBy:   c.VulcanUser,
	}

	url := fmt.Sprintf(createScanURL, c.VulcanAPI, teamID)
	operation := func() error {
		return c.performReq(http.MethodPost, url, scanMsg)
	}

	return backoff.Retry(operation, backoff.NewExponentialBackOff())
}

// SendReport triggers a report sending operation by calling vulcan-api.
func (c *VulcanClient) SendReport(teamID string) error {
	url := fmt.Sprintf(sendReportURL, c.VulcanAPI, teamID)
	operation := func() error {
		return c.performReq(http.MethodPost, url, nil)
	}

	return backoff.Retry(operation, backoff.NewExponentialBackOff())
}

func (c *VulcanClient) performReq(httpMethod, url string, payload interface{}) error {
	content, err := json.Marshal(payload)
	if err != nil {
		return &backoff.PermanentError{Err: err}
	}
	req, err := http.NewRequest(httpMethod, url, bytes.NewReader(content))
	if err != nil {
		return &backoff.PermanentError{Err: err}
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf(bearerHeaderTemplate, c.VulcanToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// This is the only error that can be
		// related to network issues, so don't
		// return a PermanentError in this case
		// so retries can be applied.
		return err
	}
	defer resp.Body.Close() // nolint

	if resp.StatusCode != http.StatusCreated {
		var content string
		b, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			content = string(b)
		}
		err = fmt.Errorf("Error. Response status %s. Content: %s", resp.Status, content)
		if resp.StatusCode >= 500 {
			// If HTTP communication was successful
			// but an error was produced in the server,
			// return non permanent err so retries
			// are applied.
			return err
		}
		return &backoff.PermanentError{
			Err: err,
		}
	}
	return nil
}
