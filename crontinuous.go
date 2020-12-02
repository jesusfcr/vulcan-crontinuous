/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"errors"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/manelmontilla/cron"
)

const (
	ScanCronType CronType = iota
	ReportCronType
)

var (
	// ErrScheduleNotFound is returned by DeleteSchedule method if the id for the schedule is not found.
	ErrScheduleNotFound = errors.New("ErrorScheduleNotFound")

	// ErrMalformedSchedule indicates the given cron spec is invalid.
	ErrMalformedSchedule = errors.New("ErrorMalformedSchedule")

	// ErrMalformedEntry indicates the given entry is invalid.
	ErrMalformedEntry = errors.New("ErrorMalformedEntry")

	// ErrInvalidCronType indicates the given cron type is invalid.
	ErrInvalidCronType = errors.New("ErrInvalidCronType")

	// errTeamNotWhitelisted is used internally from scan and report
	// cron files to indicate that entry was saved but should not be
	// created because the teamID is not whitelisted.
	errTeamNotWhitelisted = errors.New("ErrTeamNotWhitelisted")
)

// Config holds the information required by the Crontinuous
type Config struct {
	Bucket                     string
	EnableTeamsWhitelistScan   bool
	TeamsWhitelistScan         []string
	EnableTeamsWhitelistReport bool
	TeamsWhitelistReport       []string
}

type CronType int

type CronEntry interface {
	GetID() string
	GetCronSpec() string
}

type cronEntryWithSchedule struct {
	entry          CronEntry
	schedule       cron.Schedule
	overwriteEntry bool
}

type cronJobSchedule struct {
	schedule cron.Schedule
	job      cron.Job
	id       string
}

// Crontinuous implements the logic for storing and executing programs.
type Crontinuous struct {
	config Config
	log    *logrus.Logger

	scanCreator   ScanCreator
	scanCronStore ScanCronStore
	scanEntries   map[string]ScanEntry
	scanMux       sync.RWMutex

	reportSender    ReportSender
	reportCronStore ReportCronStore
	reportEntries   map[string]ReportEntry
	reportMux       sync.RWMutex

	cron *cron.Cron
}

// NewCrontinuous creates a new instance of the crontinuous service.
func NewCrontinuous(cfg Config, logger *logrus.Logger,
	scanCreator ScanCreator, scanCronStore ScanCronStore,
	reportSender ReportSender, reportCronStore ReportCronStore) *Crontinuous {

	return &Crontinuous{
		config:          cfg,
		log:             logger,
		scanCreator:     scanCreator,
		scanCronStore:   scanCronStore,
		scanEntries:     make(map[string]ScanEntry),
		reportSender:    reportSender,
		reportCronStore: reportCronStore,
		reportEntries:   make(map[string]ReportEntry),
	}
}

// Start reads the cron entries from store, s3 by now, and initializes all the entries.
func (c *Crontinuous) Start() error {
	c.cron = cron.New()

	var cronSchedules []cronJobSchedule

	// Scan Entries
	scanEntries, scanSchedules, err := c.buildScanEntries()
	if err != nil {
		return err
	}
	c.scanEntries = scanEntries
	cronSchedules = append(cronSchedules, scanSchedules...)

	// Report Entries
	reportEntries, reportSchedules, err := c.buildReportEntries()
	if err != nil {
		return err
	}
	c.reportEntries = reportEntries
	cronSchedules = append(cronSchedules, reportSchedules...)

	// Schedule cron jobs
	for _, cs := range cronSchedules {
		c.cron.Schedule(cs.schedule, cs.job, cs.id)
	}

	c.cron.Start()
	return nil
}

func (c *Crontinuous) buildScanEntries() (map[string]ScanEntry, []cronJobSchedule, error) {
	scanEntries, err := c.scanCronStore.GetScanEntries()
	if err != nil {
		return nil, nil, err
	}

	var scanSchedules []cronJobSchedule
	for _, se := range scanEntries {
		if !c.isTeamWhitelisted(ScanCronType, se.TeamID) {
			// If team is not whitelisted, return entry
			// but do not build job to be scheduled.
			continue
		}
		s, err := cron.ParseStandard(se.CronSpec)
		if err != nil {
			// Abort start
			// TODO: skip this entry and continue?
			return nil, nil, err
		}

		jobLog := logrus.New().WithFields(logrus.Fields{"job": se.ProgramID})

		scanSchedules = append(scanSchedules, cronJobSchedule{
			schedule: s,
			job: &scanJob{
				programID:   se.ProgramID,
				teamID:      se.TeamID,
				scanCreator: c.scanCreator,
				log:         jobLog,
			},
			id: se.ProgramID,
		})
	}

	return scanEntries, scanSchedules, nil
}

func (c *Crontinuous) buildReportEntries() (map[string]ReportEntry, []cronJobSchedule, error) {
	reportEntries, err := c.reportCronStore.GetReportEntries()
	if err != nil {
		return nil, nil, err
	}

	var reportSchedules []cronJobSchedule
	for _, re := range reportEntries {
		if !c.isTeamWhitelisted(ReportCronType, re.TeamID) {
			// If team is not whitelisted, return entry
			// but do not build job to be scheduled.
			continue
		}
		s, err := cron.ParseStandard(re.CronSpec)
		if err != nil {
			// Abort start
			// TODO: skip this entry and continue?
			return nil, nil, err
		}

		jobLog := logrus.New().WithFields(logrus.Fields{"job": re.TeamID})

		reportSchedules = append(reportSchedules, cronJobSchedule{
			schedule: s,
			job: &reportJob{
				teamID:       re.TeamID,
				reportSender: c.reportSender,
				log:          jobLog,
			},
			id: re.TeamID,
		})
	}

	return reportEntries, reportSchedules, nil
}

func (c *Crontinuous) isTeamWhitelisted(typ CronType, teamID string) bool {
	enable := false
	whitelist := []string{}

	if typ == ScanCronType {
		enable = c.config.EnableTeamsWhitelistScan
		whitelist = c.config.TeamsWhitelistScan
	}
	if typ == ReportCronType {
		enable = c.config.EnableTeamsWhitelistReport
		whitelist = c.config.TeamsWhitelistReport
	}

	if !enable {
		return true
	}
	for _, t := range whitelist {
		if t == teamID {
			return true
		}
	}
	return false
}

// Stop signals the command processor to stop processing commands and wait for it to exit.
func (c *Crontinuous) Stop() {
	c.cron.Stop()
	c.log.Info("Stopped")
}

// BulkCreate tests for each specified entry if an entry with the same programID exists.
// If it exists and overwrite setting for that entry is set to false the method does nothing.
// If it doesn't exist or overwrite setting is set to true, the method creates/overwrites the entry.
func (c *Crontinuous) BulkCreate(typ CronType, entries []CronEntry, overwriteSettings []bool) error {
	parsedEntries := make(map[string]cronEntryWithSchedule)

	// In order to try to reduce to the minimun the time this methods
	// locks the entries, we parse the cron strings in this loop and not inside
	// the loop below inside the lock-unlock block.
	for i, e := range entries {
		s, err := cron.ParseStandard(e.GetCronSpec())
		if err != nil {
			return ErrMalformedSchedule
		}
		parsedEntries[e.GetID()] = cronEntryWithSchedule{
			entry:          e,
			schedule:       s,
			overwriteEntry: overwriteSettings[i],
		}
	}

	var jobsWithSchedule []cronJobSchedule
	var err error

	switch typ {
	case ScanCronType:
		jobsWithSchedule, err = c.scanBulkCreate(parsedEntries)
	case ReportCronType:
		jobsWithSchedule, err = c.reportBulkCreate(parsedEntries)
	default:
		return ErrInvalidCronType
	}

	if err != nil {
		return err
	}

	for _, j := range jobsWithSchedule {
		j := j // Prevent gotcha with pointers and ranges.
		c.cron.Schedule(j.schedule, j.job, j.id)
	}
	return nil
}

// SaveEntry adds a new entry to the crontab.
func (c *Crontinuous) SaveEntry(typ CronType, entry CronEntry) error {
	s, err := cron.ParseStandard(entry.GetCronSpec())
	if err != nil {
		return ErrMalformedSchedule
	}

	var cronJob cron.Job

	switch typ {
	case ScanCronType:
		cronJob, err = c.saveScanEntry(entry)
	case ReportCronType:
		cronJob, err = c.saveReportEntry(entry)
	default:
		return ErrInvalidCronType
	}

	if err != nil {
		if errors.Is(err, errTeamNotWhitelisted) {
			// If team is not whitelisted, do not
			// schedule job and return.
			return nil
		}
		return err
	}

	c.cron.Schedule(s, cronJob, entry.GetID())
	return nil
}

// GetEntries returns a snapshot of the current entries.
func (c *Crontinuous) GetEntries(typ CronType) ([]CronEntry, error) {
	var entries []CronEntry
	var err error

	switch typ {
	case ScanCronType:
		entries, err = c.getScanEntries()
	case ReportCronType:
		entries, err = c.getReportEntries()
	default:
		return nil, ErrInvalidCronType
	}

	return entries, err
}

// GetEntryByID returns a snapshot of the current entries.
func (c *Crontinuous) GetEntryByID(typ CronType, ID string) (CronEntry, error) {
	var entry CronEntry
	var err error

	switch typ {
	case ScanCronType:
		entry, err = c.getScanEntryByID(ID)
	case ReportCronType:
		entry, err = c.getReportEntryByID(ID)
	default:
		return nil, ErrInvalidCronType
	}

	if err != nil {
		return nil, err
	}

	return entry, nil
}

// RemoveEntry remove an existing entry.
func (c *Crontinuous) RemoveEntry(typ CronType, ID string) error {
	var err error

	switch typ {
	case ScanCronType:
		err = c.removeScanEntry(ID)
	case ReportCronType:
		err = c.removeReportEntry(ID)
	default:
		return ErrInvalidCronType
	}

	if err != nil {
		return err
	}

	c.cron.RemoveJob(ID)
	return nil
}
