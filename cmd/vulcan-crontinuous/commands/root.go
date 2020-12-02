/*
Copyright 2020 Adevinta
*/

package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/julienschmidt/httprouter"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	crontinuous "github.com/adevinta/vulcan-crontinuous"
)

var (
	cfgFile string
	cfg     config
	cron    *crontinuous.Crontinuous
)

var rootCmd = &cobra.Command{
	Use:   "vulcan-crontinuous",
	Short: "Vulcanito Scan Scheduler",
	Args:  cobra.NoArgs,
	Long:  `Schedules executions of scans using cron strings`,

	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer(cfg)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.vulcan-crontinuous.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".vulcan-crontinuous" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".vulcan-crontinuous")
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("can't read config: ", err)
		os.Exit(1)
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		fmt.Printf("Can't not decode confing file %s: %s", viper.ConfigFileUsed(), err.Error())
		os.Exit(1)
	}

	if cfg.Group == "" {
		if runtime.GOOS == "darwin" {
			cfg.Group = "wheel"
		} else {
			cfg.Group = "root"
		}
	}
}

type config struct {
	HTTPPort                   int      `mapstructure:"http-port"`
	CronDir                    string   `mapstructure:"cron-dir"`
	CronScriptPath             string   `mapstructure:"cron-script-path"`
	Region                     string   `mapstructure:"region"`
	Bucket                     string   `mapstructure:"bucket"`
	AWSS3Endpoint              string   `mapstructure:"aws-s3-endpoint"`
	PathStyle                  bool     `mapstructure:"path-style"`
	Username                   string   `mapstructure:"username"`
	Group                      string   `mapstructure:"group"`
	VulcanAPI                  string   `mapstructure:"vulcan-api"`
	VulcanToken                string   `mapstructure:"vulcan-token"`
	VulcanUser                 string   `mapstructure:"vulcan-user"`
	EnableTeamsWhitelistScan   bool     `mapstructure:"enable-teams-whitelist-scan"`
	TeamsWhitelistScan         []string `mapstructure:"teams-whitelist-scan"`
	EnableTeamsWhitelistReport bool     `mapstructure:"enable-teams-whitelist-report"`
	TeamsWhitelistReport       []string `mapstructure:"teams-whitelist-report"`
}

func runServer(c config) error {
	sess, err := session.NewSession(&aws.Config{Region: &c.Region})
	if err != nil {
		log.Fatal(err)
	}
	s3Client := s3.New(sess)

	if c.AWSS3Endpoint != "" {
		s3Client = s3.New(sess, aws.NewConfig().WithEndpoint(c.AWSS3Endpoint).WithS3ForcePathStyle(c.PathStyle))
	}

	vulcanc := &crontinuous.VulcanClient{
		VulcanAPI:   c.VulcanAPI,
		VulcanToken: c.VulcanToken,
		VulcanUser:  c.VulcanUser,
	}

	s3Store := crontinuous.NewS3CronStore(c.Bucket,
		crontinuous.S3ScansCrontabFilename, crontinuous.S3ReportsCrontabFilename,
		s3Client)

	cron = crontinuous.NewCrontinuous(
		crontinuous.Config{
			Bucket:                     c.Bucket,
			EnableTeamsWhitelistScan:   c.EnableTeamsWhitelistScan,
			TeamsWhitelistScan:         c.TeamsWhitelistScan,
			EnableTeamsWhitelistReport: c.EnableTeamsWhitelistReport,
			TeamsWhitelistReport:       c.TeamsWhitelistReport,
		},
		logrus.New(),
		vulcanc, s3Store,
		vulcanc, s3Store,
	)

	err = cron.Start()
	if err != nil {
		fmt.Printf("Can not start crontinuous error: %s", err.Error())
		os.Exit(1)
	}

	router := httprouter.New()

	router.GET("/healthcheck", status)

	// Scan scheduling endpoints.
	router.GET("/entries", getScanSchedulesHandler)
	router.POST("/entries", scanBulkSettingsHandler)
	router.GET("/entries/:programID", getScanScheduleByIDHandler)
	router.DELETE("/entries/:programID", removeScanScheduleHandler)
	router.POST("/settings/:programID/:teamID", scanSettingHandler)

	// Report scheduling endpoints.
	router.GET("/report/entries", getReportSchedulesHandler)
	router.POST("/report/entries", reportBulkSettingsHandler)
	router.GET("/report/entries/:teamID", getReportScheduleByIDHandler)
	router.DELETE("/report/entries/:teamID", removeReportScheduleHandler)
	router.POST("/report/settings/:teamID", reportSettingHandler)

	addr := fmt.Sprintf(":%v", c.HTTPPort)
	fmt.Printf("Start listening at %s\n", addr)
	err = http.ListenAndServe(addr, router)
	cron.Stop()

	return err
}

type HealthcheckResponse struct {
	Status string `json:"status"`
}

func status(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	resp := HealthcheckResponse{
		Status: "OK",
	}
	encoder := json.NewEncoder(w)
	err := encoder.Encode(&resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type cronString struct {
	Str string `json:"str"`
}

type createSetting struct {
	Str       string `json:"str"`
	TeamID    string `json:"team_id"`
	ProgramID string `json:"program_id"`
	Overwrite bool   `json:"overwrite"`
}

// Bulk Settings
func scanBulkSettingsHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	settings := []createSetting{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	entries := []crontinuous.CronEntry{}
	overwriteSettings := []bool{}
	for _, s := range settings {
		entries = append(entries, crontinuous.ScanEntry{
			CronSpec:  s.Str,
			ProgramID: s.ProgramID,
			TeamID:    s.TeamID,
		})
		overwriteSettings = append(overwriteSettings, s.Overwrite)
	}

	bulkSettingsHandler(crontinuous.ScanCronType, entries, overwriteSettings, w, r, ps)
}
func reportBulkSettingsHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	settings := []createSetting{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	entries := []crontinuous.CronEntry{}
	overwriteSettings := []bool{}
	for _, s := range settings {
		entries = append(entries, crontinuous.ReportEntry{
			CronSpec: s.Str,
			TeamID:   s.TeamID,
		})
		overwriteSettings = append(overwriteSettings, s.Overwrite)
	}

	bulkSettingsHandler(crontinuous.ReportCronType, entries, overwriteSettings, w, r, ps)
}
func bulkSettingsHandler(typ crontinuous.CronType, entries []crontinuous.CronEntry, overwriteSettings []bool,
	w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	if err := cron.BulkCreate(typ, entries, overwriteSettings); err != nil {
		status := http.StatusInternalServerError
		if err == crontinuous.ErrMalformedSchedule {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
	}
}

// Setting
func scanSettingHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	programID := ps.ByName("programID")
	if programID == "" {
		http.Error(w, "Program ID missing", 400)
		return
	}
	teamID := ps.ByName("teamID")
	if teamID == "" {
		http.Error(w, "Team ID missing", 400)
		return
	}

	var c cronString
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	entry := crontinuous.ScanEntry{
		ProgramID: programID,
		TeamID:    teamID,
		CronSpec:  c.Str,
	}

	settingHandler(crontinuous.ScanCronType, entry, w, r, ps)
}
func reportSettingHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	teamID := ps.ByName("teamID")
	if teamID == "" {
		http.Error(w, "Team ID missing", 400)
		return
	}

	var c cronString
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	entry := crontinuous.ReportEntry{
		TeamID:   teamID,
		CronSpec: c.Str,
	}

	settingHandler(crontinuous.ReportCronType, entry, w, r, ps)
}
func settingHandler(typ crontinuous.CronType, entry crontinuous.CronEntry,
	w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	if err := cron.SaveEntry(typ, entry); err != nil {
		status := http.StatusInternalServerError
		if err == crontinuous.ErrMalformedSchedule {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
	}
}

// Remove Schedule
func removeScanScheduleHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("programID")
	if id == "" {
		http.Error(w, "Bad request", 400)
		return
	}

	removeScheduleHandler(crontinuous.ScanCronType, id, w, r, ps)
}
func removeReportScheduleHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("teamID")
	if id == "" {
		http.Error(w, "Bad request", 400)
		return
	}

	removeScheduleHandler(crontinuous.ReportCronType, id, w, r, ps)
}
func removeScheduleHandler(typ crontinuous.CronType, id string,
	w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	err := cron.RemoveEntry(typ, id)
	if err != nil {
		if err == crontinuous.ErrScheduleNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Get Schedules
func getScanSchedulesHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	getSchedulesHandler(crontinuous.ScanCronType, w, r, ps)
}
func getReportSchedulesHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	getSchedulesHandler(crontinuous.ReportCronType, w, r, ps)
}
func getSchedulesHandler(typ crontinuous.CronType,
	w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	entries, err := cron.GetEntries(typ)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	encoder := json.NewEncoder(w)
	err = encoder.Encode(&entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Get Schedule by ID
func getScanScheduleByIDHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("programID")
	if id == "" {
		http.Error(w, "Bad request", 400)
		return
	}

	getScheduleByIDHandler(crontinuous.ScanCronType, id, w, r, ps)
}
func getReportScheduleByIDHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("teamID")
	if id == "" {
		http.Error(w, "Bad request", 400)
		return
	}

	getScheduleByIDHandler(crontinuous.ReportCronType, id, w, r, ps)
}
func getScheduleByIDHandler(typ crontinuous.CronType, id string,
	w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

	entry, err := cron.GetEntryByID(typ, id)
	if err != nil {
		if err == crontinuous.ErrScheduleNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	encoder := json.NewEncoder(w)
	err = encoder.Encode(entry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
