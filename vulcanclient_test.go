/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	ignoreRunScanMsgDateFieldOpts = cmpopts.IgnoreFields(ScanRequest{}, "ScheduledTime")
)

func TestVulcanClient_CreateScan(t *testing.T) {
	type fields struct {
		VulcanUser  string
		VulcanToken string
	}
	tests := []struct {
		name      string
		fields    fields
		programID string
		teamID    string
		handler   func(w http.ResponseWriter, r *http.Request) string
		wantErr   bool
	}{
		{
			name: "SendsAProperCreateScanRequest",
			fields: fields{
				VulcanUser:  "user",
				VulcanToken: "token",
			},
			programID: "1",
			teamID:    "2",
			handler: func(w http.ResponseWriter, r *http.Request) string {
				if r.URL.Path != "/v1/teams/2/scans" {
					return "wrong path:" + r.URL.Path
				}
				s := ScanRequest{}
				d := json.NewDecoder(r.Body)
				err := d.Decode(&s)
				if err != nil {
					return err.Error()
				}
				diff := cmp.Diff(s, ScanRequest{
					ProgramID:   "1",
					RequestedBy: "user",
				}, ignoreRunScanMsgDateFieldOpts)
				if diff == "" {
					w.WriteHeader(http.StatusCreated)
				}
				return diff
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diff string
			s := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					diff = tt.handler(w, r)
				}))
			defer s.Close()

			c := &VulcanClient{
				VulcanAPI:   s.URL,
				VulcanUser:  tt.fields.VulcanUser,
				VulcanToken: tt.fields.VulcanToken,
			}
			err := c.CreateScan(tt.programID, tt.teamID)
			if (err != nil) != tt.wantErr {
				t.Errorf("VulcanClient.CreateScan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff != "" {
				t.Errorf(diff)
			}
		})
	}
}

func TestVulcanClient_SendReport(t *testing.T) {
	type fields struct {
		VulcanUser  string
		VulcanToken string
	}
	tests := []struct {
		name    string
		fields  fields
		teamID  string
		handler func(w http.ResponseWriter, r *http.Request) string
		wantErr bool
	}{
		{
			name: "SendsAProperSendReportRequest",
			fields: fields{
				VulcanUser:  "user",
				VulcanToken: "token",
			},
			teamID: "2",
			handler: func(w http.ResponseWriter, r *http.Request) string {
				if r.URL.Path != "/v1/teams/2/report/digest" {
					return "wrong path:" + r.URL.Path
				}
				w.WriteHeader(http.StatusCreated)
				return ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diff string
			s := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					diff = tt.handler(w, r)
				}))
			defer s.Close()

			c := &VulcanClient{
				VulcanAPI:   s.URL,
				VulcanUser:  tt.fields.VulcanUser,
				VulcanToken: tt.fields.VulcanToken,
			}
			err := c.SendReport(tt.teamID)
			if (err != nil) != tt.wantErr {
				t.Errorf("VulcanClient.SendReport() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff != "" {
				t.Errorf(diff)
			}
		})
	}
}

func TestVulcanClient_TestBackoff(t *testing.T) {
	// Variable used to count consecutive requests
	// to handler
	var reqCounter uint

	type fields struct {
		VulcanUser  string
		VulcanToken string
	}
	tests := []struct {
		name    string
		fields  fields
		teamID  string
		handler func(w http.ResponseWriter, r *http.Request) string
		wantErr bool
	}{
		{
			name: "SendsAProperSendReportRequest",
			fields: fields{
				VulcanUser:  "user",
				VulcanToken: "token",
			},
			teamID: "3",
			handler: func(w http.ResponseWriter, r *http.Request) string {
				reqCounter++
				if r.URL.Path != "/v1/teams/3/report/digest" {
					return "wrong path:" + r.URL.Path
				}
				if reqCounter == 1 {
					// If it's first request, return 500 err
					// to test backoff implementation
					w.WriteHeader(http.StatusInternalServerError)
					return "500"
				}
				w.WriteHeader(http.StatusCreated)
				return ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diff string
			s := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					diff = tt.handler(w, r)
				}))
			defer s.Close()

			c := &VulcanClient{
				VulcanAPI:   s.URL,
				VulcanUser:  tt.fields.VulcanUser,
				VulcanToken: tt.fields.VulcanToken,
			}
			err := c.SendReport(tt.teamID)
			if (err != nil) != tt.wantErr {
				t.Errorf("VulcanClient.SendReport() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff != "" {
				t.Errorf(diff)
			}
		})
	}
}
