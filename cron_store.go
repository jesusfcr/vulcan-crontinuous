/*
Copyright 2020 Adevinta
*/

package crontinuous

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

var (
	errEntriesFileNotFound = errors.New("EntriesFileNotFound")
)

type ScanCronStore interface {
	GetScanEntries() (map[string]ScanEntry, error)
	SaveScanEntries(entries map[string]ScanEntry) error
}

type ReportCronStore interface {
	GetReportEntries() (map[string]ReportEntry, error)
	SaveReportEntries(entries map[string]ReportEntry) error
}

type S3CronStore struct {
	bucket        string
	scanCronKey   string
	reportCronKey string
	s3Client      s3iface.S3API
}

func NewS3CronStore(bucket, scanCronKey, reportCronKey string, s3Client s3iface.S3API) *S3CronStore {
	return &S3CronStore{
		bucket:        bucket,
		scanCronKey:   scanCronKey,
		reportCronKey: reportCronKey,
		s3Client:      s3Client,
	}
}

func (s *S3CronStore) GetScanEntries() (map[string]ScanEntry, error) {
	entriesData, err := s.getEntriesData(s.scanCronKey)
	if err != nil {
		// If entries file is not found
		// return void entries map.
		//
		// This allows to auto create the entries file
		// automatically in remote store when a new entry
		// is added via API.
		if err == errEntriesFileNotFound {
			return map[string]ScanEntry{}, nil
		}
		return nil, err
	}

	var scanEntries map[string]ScanEntry
	err = json.Unmarshal(entriesData, &scanEntries)
	return scanEntries, err
}

func (s *S3CronStore) SaveScanEntries(entries map[string]ScanEntry) error {
	return s.saveEntries(s.scanCronKey, entries)
}

func (s *S3CronStore) GetReportEntries() (map[string]ReportEntry, error) {
	entriesData, err := s.getEntriesData(s.reportCronKey)
	if err != nil {
		// If entries file is not found
		// return void entries map.
		//
		// This allows to auto create the entries file
		// automatically in remote store when a new entry
		// is added via API.
		if err == errEntriesFileNotFound {
			return map[string]ReportEntry{}, nil
		}
		return nil, err
	}

	var reportEntries map[string]ReportEntry
	err = json.Unmarshal(entriesData, &reportEntries)
	return reportEntries, err
}

func (s *S3CronStore) SaveReportEntries(entries map[string]ReportEntry) error {
	return s.saveEntries(s.reportCronKey, entries)
}

func (s *S3CronStore) getEntriesData(key string) ([]byte, error) {
	output, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				return nil, errEntriesFileNotFound
			default:
				return nil, err
			}
		}
		return nil, err
	}

	return ioutil.ReadAll(output.Body)
}

func (s *S3CronStore) saveEntries(key string, entries interface{}) error {
	content, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(content),
	}
	_, err = s.s3Client.PutObject(params)
	return err
}
