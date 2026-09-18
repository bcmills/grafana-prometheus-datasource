package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type File struct {
	Seed     int64              `yaml:"seed"`
	Scales   map[string]int     `yaml:"scales"`
	Grafana  Grafana            `yaml:"grafana"`
	Profiles map[string]Profile `yaml:"profiles"`
}

type Grafana struct {
	SearchUID string `yaml:"search_uid"`
	LabelsUID string `yaml:"labels_uid"`
}

type Profile struct {
	Scales         []string `yaml:"scales"`
	Shapes         []string `yaml:"shapes"`
	Storage        []string `yaml:"storage"`
	Windows        []string `yaml:"windows"`
	Endpoints      []string `yaml:"endpoints"`
	QueryKinds     []string `yaml:"query_kinds"`
	Selectors      []string `yaml:"selectors"`
	Limits         []int    `yaml:"limits"`
	BatchSizes     []int    `yaml:"batch_sizes"`
	Concurrency    []int    `yaml:"concurrency"`
	Cancel         []string `yaml:"cancel"`
	Networks       []string `yaml:"networks"`
	Modes          []string `yaml:"modes"`
	CacheClasses   []string `yaml:"cache_classes"`
	Warmups        int      `yaml:"warmups"`
	MeasuredTrials int      `yaml:"measured_trials"`
}

func Load(path string) (*File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file File
	if err := yaml.Unmarshal(body, &file); err != nil {
		return nil, err
	}
	if len(file.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles in %s", path)
	}
	return &file, nil
}

func (f *File) Profile(name string) (Profile, error) {
	profile, ok := f.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown profile %q", name)
	}
	if profile.Warmups == 0 {
		profile.Warmups = 1
	}
	if profile.MeasuredTrials == 0 {
		profile.MeasuredTrials = 1
	}
	return profile, nil
}
