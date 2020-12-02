/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/manelmontilla/cron"
)

var (
	sortEntriesSliceOption = cmp.Transformer("SortEntries", func(in []CronEntry) []CronEntry {
		out := append([]CronEntry(nil), in...)
		sort.Slice(out, func(i, j int) bool {
			return strings.Compare(out[i].GetID(), out[j].GetID()) < 0
		})
		return out
	})
	sortJobsSliceOption = cmp.Transformer("SortJobs", func(in []*cron.Entry) []*cron.Entry {
		out := append([]*cron.Entry(nil), in...)
		sort.Slice(out, func(i, j int) bool {
			return strings.Compare(out[i].ID, out[j].ID) < 0
		})
		return out
	})
)

type mockCronStore struct {
	ScanCronStore
	ReportCronStore
	scanEntries   map[string]ScanEntry
	reportEntries map[string]ReportEntry
}

func (s *mockCronStore) GetScanEntries() (map[string]ScanEntry, error) {
	return s.scanEntries, nil
}
func (s *mockCronStore) SaveScanEntries(entries map[string]ScanEntry) error {
	s.scanEntries = entries
	return nil
}
func (s *mockCronStore) GetReportEntries() (map[string]ReportEntry, error) {
	return s.reportEntries, nil
}
func (s *mockCronStore) SaveReportEntries(entries map[string]ReportEntry) error {
	s.reportEntries = entries
	return nil
}

type mockScanCreator struct {
	creator func(string, string) error
}

func (m *mockScanCreator) CreateScan(programID, teamID string) error {
	return m.creator(programID, teamID)
}

type mockReportSender struct {
	sender func(string) error
}

func (m *mockReportSender) SendReport(teamID string) error {
	return m.sender(teamID)
}

// This test takes ~4min to run due to cron's min
// preiodicity to be 1min, which is a pain but this way
// we test for real execution and not some mocking of
// inner cron object.
func TestExecutesEntries(t *testing.T) {
	// var used to track completion
	// of scheduled jobs. This flag
	// will be set to true when a
	// CreateScan or SendReport actions
	// are triggered.
	var jobRunFlag bool

	flagSwitcherScanCreator := &mockScanCreator{
		creator: func(string, string) error {
			jobRunFlag = true
			return nil
		},
	}
	flagSwitcherReportSender := &mockReportSender{
		sender: func(string) error {
			jobRunFlag = true
			return nil
		},
	}

	type fields struct {
		config          Config
		scanCreator     ScanCreator
		scanCronStore   ScanCronStore
		reportSender    ReportSender
		reportCronStore ReportCronStore
	}

	testCases := []struct {
		name           string
		fields         fields
		wantJobRunFlag bool
	}{
		{
			name: "Should execute ScanJob",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   false,
					EnableTeamsWhitelistReport: false,
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{
						"progID": {
							ProgramID: "progID",
							TeamID:    "teamID",
							CronSpec:  "* * * * *",
						},
					},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{},
				},
			},
			wantJobRunFlag: true,
		},
		{
			name: "Should execute ScanJob, whitelist for reports is enabled",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   false,
					EnableTeamsWhitelistReport: true,
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{
						"progID": {
							ProgramID: "progID",
							TeamID:    "teamID",
							CronSpec:  "* * * * *",
						},
					},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{},
				},
			},
			wantJobRunFlag: true,
		},
		{
			name: "Should execute ReportJob",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   false,
					EnableTeamsWhitelistReport: false,
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{
						"teamID": {
							TeamID:   "teamID",
							CronSpec: "* * * * *",
						},
					},
				},
			},
			wantJobRunFlag: true,
		},
		{
			name: "Should execute ReportJob, whitelist for scans is enabled",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   true,
					EnableTeamsWhitelistReport: false,
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{
						"teamID": {
							TeamID:   "teamID",
							CronSpec: "* * * * *",
						},
					},
				},
			},
			wantJobRunFlag: true,
		},
		{
			name: "Should not execute ReportJob, team not scheduled",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   true,
					TeamsWhitelistScan:         []string{"AnotherTeam"},
					EnableTeamsWhitelistReport: true,
					TeamsWhitelistReport:       []string{"AnotherTeam"},
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{
						"teamID": {
							TeamID:   "teamID",
							CronSpec: "* * * * *",
						},
					},
				},
			},
			wantJobRunFlag: false,
		},
		{
			name: "Should not execute ReportJob, team not scheduled for Report, but team is whitelisted for Scan",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   true,
					TeamsWhitelistScan:         []string{"teamID"},
					EnableTeamsWhitelistReport: true,
					TeamsWhitelistReport:       []string{"AnotherTeam"},
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{
						"teamID": {
							TeamID:   "teamID",
							CronSpec: "* * * * *",
						},
					},
				},
			},
			wantJobRunFlag: false,
		},
		{
			name: "Should not execute ScanJob, team not whitelisted",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   true,
					TeamsWhitelistScan:         []string{"AnotherTeam"},
					EnableTeamsWhitelistReport: true,
					TeamsWhitelistReport:       []string{"AnotherTeam"},
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{
						"progID": {
							ProgramID: "progID",
							TeamID:    "teamID",
							CronSpec:  "* * * * *",
						},
					},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{},
				},
			},
			wantJobRunFlag: false,
		},
		{
			name: "Should not execute ScanJob, team not whitelisted for scan, but team is whitelisted for report",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   true,
					TeamsWhitelistScan:         []string{"AnotherTeam"},
					EnableTeamsWhitelistReport: true,
					TeamsWhitelistReport:       []string{"teamID"},
				},
				scanCreator: flagSwitcherScanCreator,
				scanCronStore: &mockCronStore{
					scanEntries: map[string]ScanEntry{
						"progID": {
							ProgramID: "progID",
							TeamID:    "teamID",
							CronSpec:  "* * * * *",
						},
					},
				},
				reportSender: flagSwitcherReportSender,
				reportCronStore: &mockCronStore{
					reportEntries: map[string]ReportEntry{},
				},
			},
			wantJobRunFlag: false,
		},
	}

	for _, tc := range testCases {
		// reset flag
		jobRunFlag = false

		t.Run(tc.name, func(*testing.T) {
			c := NewCrontinuous(tc.fields.config, logrus.New(),
				tc.fields.scanCreator, tc.fields.scanCronStore,
				tc.fields.reportSender, tc.fields.reportCronStore)

			err := c.Start()
			if err != nil {
				t.Fatalf("Error starting crontinuous: %v", err)
			}

			// Wait for job to finish
			<-time.After(1*time.Minute + 500*time.Millisecond)
			c.Stop()

			if jobRunFlag != tc.wantJobRunFlag {
				t.Fatalf("Error, expected job to be %v, but it was not", tc.wantJobRunFlag)
			}
		})
	}
}

func TestCrontinuous_GetEntries(t *testing.T) {
	tests := []struct {
		name              string
		scanEntries       map[string]ScanEntry
		reportEntries     map[string]ReportEntry
		wantScanEntries   []CronEntry
		wantReportEntries []CronEntry
	}{
		{
			name: "Happy path",
			scanEntries: map[string]ScanEntry{
				"1": {
					CronSpec:  "*/2 * * * *",
					ProgramID: "1",
					TeamID:    "team1",
				},
				"2": {
					CronSpec:  "*/3 * * * *",
					ProgramID: "2",
					TeamID:    "team2",
				},
			},
			reportEntries: map[string]ReportEntry{
				"a": {
					TeamID:   "a",
					CronSpec: "*/5 * * * *",
				},
				"b": {
					TeamID:   "b",
					CronSpec: "*/10 * * * 1",
				},
			},
			wantScanEntries: []CronEntry{
				ScanEntry{
					CronSpec:  "*/2 * * * *",
					ProgramID: "1",
					TeamID:    "team1",
				},
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "2",
					TeamID:    "team2",
				},
			},
			wantReportEntries: []CronEntry{
				ReportEntry{
					TeamID:   "a",
					CronSpec: "*/5 * * * *",
				},
				ReportEntry{
					TeamID:   "b",
					CronSpec: "*/10 * * * 1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Crontinuous{
				scanEntries:   tt.scanEntries,
				reportEntries: tt.reportEntries,
			}

			gotScanEntries, err := c.GetEntries(ScanCronType)
			if err != nil {
				t.Fatalf("Error retrieving entries: %v", err)
			}
			diffs := cmp.Diff(tt.wantScanEntries, gotScanEntries, cmp.Options{sortEntriesSliceOption})
			if diffs != "" {
				t.Fatalf("wantScanEntries != got. %s", diffs)
			}

			gotReportEntries, err := c.GetEntries(ReportCronType)
			if err != nil {
				t.Fatalf("Error retrieving entries: %v", err)
			}
			diffs = cmp.Diff(tt.wantReportEntries, gotReportEntries, cmp.Options{sortEntriesSliceOption})
			if diffs != "" {
				t.Fatalf("wantReportEntries != got. %s", diffs)
			}
		})
	}
}

func TestCrontinuous_BulkCreate(t *testing.T) {
	type fields struct {
		config          Config
		scanCronStore   ScanCronStore
		scanEntries     map[string]ScanEntry
		reportCronStore ReportCronStore
		reportEntries   map[string]ReportEntry
	}

	mockCronStore := &mockCronStore{}

	tests := []struct {
		name                    string
		fields                  fields
		inputScanEntries        []CronEntry
		scanOverwriteSettings   []bool
		wantScanEntries         map[string]ScanEntry
		inputReportEntries      []CronEntry
		reportOverwriteSettings []bool
		wantReportEntries       map[string]ReportEntry
		wantJobs                []*cron.Entry
	}{
		{
			name: "HappyPath",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   false,
					EnableTeamsWhitelistReport: false,
				},
				scanCronStore: mockCronStore,
				scanEntries: map[string]ScanEntry{
					"scanScheduled": {
						CronSpec:  "*/2 * * * *",
						ProgramID: "scanScheduled",
						TeamID:    "ateam",
					},
					"scanOverwritable": {
						CronSpec:  "*/4 * * * *",
						ProgramID: "scanOverwritable",
						TeamID:    "someTeam",
					},
				},
				reportCronStore: mockCronStore,
				reportEntries: map[string]ReportEntry{
					"reportScheduled": {
						CronSpec: "*/5 * * * *",
						TeamID:   "reportScheduled",
					},
					"reportOverwritable": {
						CronSpec: "*/6 * * * *",
						TeamID:   "reportOverwritable",
					},
				},
			},
			inputScanEntries: []CronEntry{
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				ScanEntry{
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			scanOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantScanEntries: map[string]ScanEntry{
				"scanScheduled": {
					CronSpec:  "*/2 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				"newProgram": {
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				"scanOverwritable": {
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			inputReportEntries: []CronEntry{
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam",
				},
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "reportScheduled",
				},
				ReportEntry{
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			reportOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantReportEntries: map[string]ReportEntry{
				"otherteam": {
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam",
				},
				"reportScheduled": {
					CronSpec: "*/5 * * * *",
					TeamID:   "reportScheduled",
				},
				"reportOverwritable": {
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			wantJobs: []*cron.Entry{
				{
					ID:       "scanScheduled",
					Schedule: mustParseSchedule("*/2 * * * *"),
				},
				{
					ID:       "newProgram",
					Schedule: mustParseSchedule("*/3 * * * *"),
				},
				{
					ID:       "scanOverwritable",
					Schedule: mustParseSchedule("*/5 * * * *"),
				},
				{
					ID:       "otherteam",
					Schedule: mustParseSchedule("*/3 * * * *"),
				},
				{
					ID:       "reportScheduled",
					Schedule: mustParseSchedule("*/5 * * * *"),
				},
				{
					ID:       "reportOverwritable",
					Schedule: mustParseSchedule("*/7 * * * *"),
				},
			},
		},
		{
			name: "WhitelistedTeamsScan",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan: true,
					TeamsWhitelistScan: []string{
						"ateam",
						"someteam",
						"reportScheduled",
						"otherteam",
						"reportOverwritable",
					},
					EnableTeamsWhitelistReport: false,
				},
				scanCronStore: mockCronStore,
				scanEntries: map[string]ScanEntry{
					"scanScheduled": {
						CronSpec:  "*/2 * * * *",
						ProgramID: "scanScheduled",
						TeamID:    "ateam",
					},
					"scanOverwritable": {
						CronSpec:  "*/4 * * * *",
						ProgramID: "scanOverwritable",
						TeamID:    "someTeam",
					},
				},
				reportCronStore: mockCronStore,
				reportEntries: map[string]ReportEntry{
					"reportScheduled": {
						CronSpec: "*/5 * * * *",
						TeamID:   "reportScheduled",
					},
					"reportOverwritable": {
						CronSpec: "*/6 * * * *",
						TeamID:   "reportOverwritable",
					},
				},
			},
			inputScanEntries: []CronEntry{
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				ScanEntry{
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			scanOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantScanEntries: map[string]ScanEntry{
				"scanScheduled": {
					CronSpec:  "*/2 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				"newProgram": {
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				"scanOverwritable": {
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			inputReportEntries: []CronEntry{
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam2",
				},
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "reportScheduled",
				},
				ReportEntry{
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			reportOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantReportEntries: map[string]ReportEntry{
				"otherteam2": {
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam2",
				},
				"reportScheduled": {
					CronSpec: "*/5 * * * *",
					TeamID:   "reportScheduled",
				},
				"reportOverwritable": {
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			wantJobs: []*cron.Entry{
				{
					ID:       "scanScheduled",
					Schedule: mustParseSchedule("*/2 * * * *"),
				},
				{
					ID:       "newProgram",
					Schedule: mustParseSchedule("*/3 * * * *"),
				},
				{
					ID:       "otherteam2",
					Schedule: mustParseSchedule("*/3 * * * *"),
				},
				{
					ID:       "scanOverwritable",
					Schedule: mustParseSchedule("*/4 * * * *"),
				},
				{
					ID:       "reportScheduled",
					Schedule: mustParseSchedule("*/5 * * * *"),
				},
				{
					ID:       "reportOverwritable",
					Schedule: mustParseSchedule("*/7 * * * *"),
				},
			},
		},
		{
			name: "WhitelistedTeamsReport",
			fields: fields{
				config: Config{
					EnableTeamsWhitelistScan:   false,
					EnableTeamsWhitelistReport: true,
					TeamsWhitelistReport: []string{
						"ateam",
						"someteam",
						"reportScheduled",
						"otherteam",
						"reportOverwritable",
					},
				},
				scanCronStore: mockCronStore,
				scanEntries: map[string]ScanEntry{
					"scanScheduled": {
						CronSpec:  "*/2 * * * *",
						ProgramID: "scanScheduled",
						TeamID:    "ateam",
					},
					"scanOverwritable": {
						CronSpec:  "*/4 * * * *",
						ProgramID: "scanOverwritable",
						TeamID:    "someTeam",
					},
				},
				reportCronStore: mockCronStore,
				reportEntries: map[string]ReportEntry{
					"reportScheduled": {
						CronSpec: "*/5 * * * *",
						TeamID:   "reportScheduled",
					},
					"reportOverwritable": {
						CronSpec: "*/6 * * * *",
						TeamID:   "reportOverwritable",
					},
				},
			},
			inputScanEntries: []CronEntry{
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				ScanEntry{
					CronSpec:  "*/3 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				ScanEntry{
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			scanOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantScanEntries: map[string]ScanEntry{
				"scanScheduled": {
					CronSpec:  "*/2 * * * *",
					ProgramID: "scanScheduled",
					TeamID:    "ateam",
				},
				"newProgram": {
					CronSpec:  "*/3 * * * *",
					ProgramID: "newProgram",
					TeamID:    "otherteam",
				},
				"scanOverwritable": {
					CronSpec:  "*/5 * * * *",
					ProgramID: "scanOverwritable",
					TeamID:    "someTeam",
				},
			},
			inputReportEntries: []CronEntry{
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam2",
				},
				ReportEntry{
					CronSpec: "*/3 * * * *",
					TeamID:   "reportScheduled",
				},
				ReportEntry{
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			reportOverwriteSettings: []bool{
				false,
				false,
				true,
			},
			wantReportEntries: map[string]ReportEntry{
				"otherteam2": {
					CronSpec: "*/3 * * * *",
					TeamID:   "otherteam2",
				},
				"reportScheduled": {
					CronSpec: "*/5 * * * *",
					TeamID:   "reportScheduled",
				},
				"reportOverwritable": {
					CronSpec: "*/7 * * * *",
					TeamID:   "reportOverwritable",
				},
			},
			wantJobs: []*cron.Entry{
				{
					ID:       "scanScheduled",
					Schedule: mustParseSchedule("*/2 * * * *"),
				},
				{
					ID:       "newProgram",
					Schedule: mustParseSchedule("*/3 * * * *"),
				},
				{
					ID:       "scanOverwritable",
					Schedule: mustParseSchedule("*/5 * * * *"),
				},
				{
					ID:       "reportScheduled",
					Schedule: mustParseSchedule("*/5 * * * *"),
				},
				{
					ID:       "reportOverwritable",
					Schedule: mustParseSchedule("*/7 * * * *"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Crontinuous{
				config:          tt.fields.config,
				log:             logrus.New(),
				scanCronStore:   tt.fields.scanCronStore,
				scanEntries:     tt.fields.scanEntries,
				reportCronStore: tt.fields.reportCronStore,
				reportEntries:   tt.fields.reportEntries,
				cron:            cron.New(),
			}

			// Add initial entries to crontab so we verify
			// later on that the correct entries are scheduled.
			for _, e := range tt.fields.scanEntries {
				s := mustParseSchedule(e.GetCronSpec())
				c.cron.Schedule(s, &voidCronJob{}, e.GetID())
			}
			for _, e := range tt.fields.reportEntries {
				s := mustParseSchedule(e.GetCronSpec())
				c.cron.Schedule(s, &voidCronJob{}, e.GetID())
			}

			// Scan Entries
			err := c.BulkCreate(ScanCronType, tt.inputScanEntries, tt.scanOverwriteSettings)
			if err != nil {
				t.Fatalf("Error Scan BulkCreate: %v", err)
			}
			diff := cmp.Diff(c.scanEntries, tt.wantScanEntries)
			if diff != "" {
				t.Fatalf("scan entries got!=want, diff %s", diff)
			}
			diff = cmp.Diff(mockCronStore.scanEntries, tt.wantScanEntries)
			if diff != "" {
				t.Fatalf("saved scan entries != want, diff %s", diff)
			}

			// Report Entries
			err = c.BulkCreate(ReportCronType, tt.inputReportEntries, tt.reportOverwriteSettings)
			if err != nil {
				t.Fatalf("Error Report BulkCreate: %v", err)
			}
			diff = cmp.Diff(c.reportEntries, tt.wantReportEntries)
			if diff != "" {
				t.Fatalf("report entries got!=want, diff %s", diff)
			}
			diff = cmp.Diff(mockCronStore.reportEntries, tt.wantReportEntries)
			if diff != "" {
				t.Fatalf("saved report entries != want, diff %s", diff)
			}

			// Jobs
			if tt.wantJobs != nil {
				got := c.cron.Entries()
				diff := cmp.Diff(got, tt.wantJobs, sortJobsSliceOption, cmpopts.IgnoreFields(cron.Entry{}, "Job"))
				if diff != "" {
					t.Errorf("jobs got!=want, diff %s", diff)
				}
			}
		})
	}
}

type voidCronJob struct{}

func (j *voidCronJob) Run() {}

func mustParseSchedule(cronexpr string) cron.Schedule {
	s, err := cron.ParseStandard(cronexpr)
	if err != nil {
		panic(err)
	}
	return s
}
