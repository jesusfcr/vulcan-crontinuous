/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"github.com/Sirupsen/logrus"
	"github.com/manelmontilla/cron"
)

const (
	S3ScansCrontabFilename = "crontab.json"
)

// ScanCreator defines the services needed by the crontinuos component
// in order to create scans.
type ScanCreator interface {
	CreateScan(scanID, teamID string) error
}

// ScanEntry defines the data stored by a scan cron entry.
type ScanEntry struct {
	ProgramID string `json:"program_id"`
	TeamID    string `json:"team_id"`
	CronSpec  string `json:"cron_spec"`
}

func (e ScanEntry) GetID() string {
	return e.ProgramID
}
func (e ScanEntry) GetCronSpec() string {
	return e.CronSpec
}

type scanJob struct {
	programID   string
	teamID      string
	scanCreator ScanCreator
	log         *logrus.Entry
}

func (j *scanJob) Run() {
	j.log.Info("Executing Scan Job")
	err := j.scanCreator.CreateScan(j.programID, j.teamID)
	if err != nil {
		j.log.Error("Error Executing Scan Job", err)
		return
	}
	j.log.Info("Executed Scan Job")
}

func (c *Crontinuous) scanBulkCreate(scheduledEntries map[string]cronEntryWithSchedule) ([]cronJobSchedule, error) {
	c.scanMux.Lock()
	defer c.scanMux.Unlock()

	// Make deep copy of current jobs in order
	// to make the operation atomic.
	current := make(map[string]ScanEntry)
	for _, e := range c.scanEntries {
		current[e.ProgramID] = e
	}

	// Update the hash of entries and create required jobs to be scheduled.
	scheduledJobs := []cronJobSchedule{}
	for _, e := range scheduledEntries {
		var se ScanEntry
		var ok bool

		if se, ok = e.entry.(ScanEntry); !ok {
			return nil, ErrMalformedEntry
		}

		if _, ok := current[se.ProgramID]; ok && !e.overwriteEntry {
			continue
		}

		current[se.ProgramID] = se

		if !c.isTeamWhitelisted(ScanCronType, se.TeamID) {
			// If team is not whitelisted, do not
			// return job to schedule.
			continue
		}

		jobLog := logrus.New().WithFields(logrus.Fields{"job": se.ProgramID})
		scheduledJobs = append(scheduledJobs, cronJobSchedule{
			schedule: e.schedule,
			job: &scanJob{
				scanCreator: c.scanCreator,
				programID:   se.ProgramID,
				teamID:      se.TeamID,
				log:         jobLog,
			},
			id: se.ProgramID,
		})
	}

	// Now it's safe to update all the entries and reschedule the jobs.
	c.scanEntries = current
	err := c.scanCronStore.SaveScanEntries(c.scanEntries)
	return scheduledJobs, err
}

func (c *Crontinuous) saveScanEntry(entry CronEntry) (cron.Job, error) {
	scanEntry, ok := entry.(ScanEntry)
	if !ok {
		return nil, ErrMalformedEntry
	}

	c.scanMux.Lock()
	defer c.scanMux.Unlock()

	c.scanEntries[scanEntry.ProgramID] = scanEntry

	err := c.scanCronStore.SaveScanEntries(c.scanEntries)
	if err != nil {
		return nil, err
	}

	if !c.isTeamWhitelisted(ScanCronType, scanEntry.TeamID) {
		return nil, errTeamNotWhitelisted
	}

	jobLog := logrus.New().WithFields(logrus.Fields{"job": scanEntry.ProgramID})

	return &scanJob{
		scanCreator: c.scanCreator,
		programID:   scanEntry.ProgramID,
		teamID:      scanEntry.TeamID,
		log:         jobLog,
	}, nil
}

func (c *Crontinuous) getScanEntries() ([]CronEntry, error) {
	c.scanMux.RLock()
	defer c.scanMux.RUnlock()

	var entries = []CronEntry{}
	for _, e := range c.scanEntries {
		entries = append(entries, e)
	}

	return entries, nil
}

func (c *Crontinuous) getScanEntryByID(ID string) (ScanEntry, error) {
	c.scanMux.RLock()
	defer c.scanMux.RUnlock()

	e, ok := c.scanEntries[ID]
	if !ok {
		return ScanEntry{}, ErrScheduleNotFound
	}

	return e, nil
}

func (c *Crontinuous) removeScanEntry(ID string) error {
	c.scanMux.Lock()
	defer c.scanMux.Unlock()

	_, ok := c.scanEntries[ID]
	if !ok {
		return ErrScheduleNotFound
	}
	delete(c.scanEntries, ID)

	return c.scanCronStore.SaveScanEntries(c.scanEntries)
}
