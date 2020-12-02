/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"github.com/Sirupsen/logrus"
	"github.com/manelmontilla/cron"
)

const (
	S3ReportsCrontabFilename = "reportsCrontab.json"
)

// ReportSender defines the service needed by the crontinuos component
// in order to trigger digest reports generation and sending.
type ReportSender interface {
	SendReport(teamID string) error
}

// ReportEntry defines the data stored by a report cron entry.
type ReportEntry struct {
	TeamID   string `json:"team_id"`
	CronSpec string `json:"cron_spec"`
}

func (e ReportEntry) GetID() string {
	return e.TeamID
}
func (e ReportEntry) GetCronSpec() string {
	return e.CronSpec
}

type reportJob struct {
	teamID       string
	reportSender ReportSender
	log          *logrus.Entry
}

func (j *reportJob) Run() {
	j.log.Info("Executing Report Job")
	err := j.reportSender.SendReport(j.teamID)
	if err != nil {
		j.log.Error("Error Executing Report Job", err)
		return
	}
	j.log.Info("Executed Report Job")
}

func (c *Crontinuous) reportBulkCreate(scheduledEntries map[string]cronEntryWithSchedule) ([]cronJobSchedule, error) {
	c.reportMux.Lock()
	defer c.reportMux.Unlock()

	// Make deep copy of current jobs in order
	// to make the operation atomic.
	current := make(map[string]ReportEntry)
	for _, e := range c.reportEntries {
		current[e.TeamID] = e
	}

	// Update the hash of entries and create required jobs to be scheduled.
	scheduledJobs := []cronJobSchedule{}
	for _, e := range scheduledEntries {
		var re ReportEntry
		var ok bool

		if re, ok = e.entry.(ReportEntry); !ok {
			return nil, ErrMalformedEntry
		}

		if _, ok := current[re.TeamID]; ok && !e.overwriteEntry {
			continue
		}

		current[re.TeamID] = re

		if !c.isTeamWhitelisted(ReportCronType, re.TeamID) {
			// If team is not whitelisted, do not
			// return job to schedule.
			continue
		}

		jobLog := logrus.New().WithFields(logrus.Fields{"job": re.TeamID})
		scheduledJobs = append(scheduledJobs, cronJobSchedule{
			schedule: e.schedule,
			job: &reportJob{
				reportSender: c.reportSender,
				teamID:       re.TeamID,
				log:          jobLog,
			},
			id: re.TeamID,
		})
	}

	// Now it's safe to update all the entries and reschedule the jobs.
	c.reportEntries = current
	err := c.reportCronStore.SaveReportEntries(c.reportEntries)
	return scheduledJobs, err
}

func (c *Crontinuous) saveReportEntry(entry CronEntry) (cron.Job, error) {
	reportEntry, ok := entry.(ReportEntry)
	if !ok {
		return nil, ErrMalformedEntry
	}

	c.reportMux.Lock()
	defer c.reportMux.Unlock()

	c.reportEntries[reportEntry.TeamID] = reportEntry

	err := c.reportCronStore.SaveReportEntries(c.reportEntries)
	if err != nil {
		return nil, err
	}

	if !c.isTeamWhitelisted(ReportCronType, reportEntry.TeamID) {
		return nil, errTeamNotWhitelisted
	}

	jobLog := logrus.New().WithFields(logrus.Fields{"job": reportEntry.TeamID})

	return &reportJob{
		teamID:       reportEntry.TeamID,
		reportSender: c.reportSender,
		log:          jobLog,
	}, nil
}

func (c *Crontinuous) getReportEntries() ([]CronEntry, error) {
	c.reportMux.RLock()
	defer c.reportMux.RUnlock()

	var entries = []CronEntry{}
	for _, e := range c.reportEntries {
		entries = append(entries, e)
	}

	return entries, nil
}

func (c *Crontinuous) getReportEntryByID(ID string) (ReportEntry, error) {
	c.reportMux.RLock()
	defer c.reportMux.RUnlock()

	e, ok := c.reportEntries[ID]
	if !ok {
		return ReportEntry{}, ErrScheduleNotFound
	}

	return e, nil
}

func (c *Crontinuous) removeReportEntry(ID string) error {
	c.reportMux.Lock()
	defer c.reportMux.Unlock()

	_, ok := c.reportEntries[ID]
	if !ok {
		return ErrScheduleNotFound
	}
	delete(c.reportEntries, ID)

	return c.reportCronStore.SaveReportEntries(c.reportEntries)
}
