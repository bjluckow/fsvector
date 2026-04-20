package cluster

import (
	"os"

	"gopkg.in/yaml.v3"
)

type GlobalConfig struct {
	Threshold     float64 `yaml:"threshold"`
	TopCategories int     `yaml:"top_categories"`
	Uncategorized bool    `yaml:"uncategorized"`
}

type Category struct {
	Name        string   `yaml:"name"`
	Threshold   float64  `yaml:"threshold"`
	Labels      []string `yaml:"labels"`
	PathSignals []string `yaml:"path_signals"`
}

type CategoriesFile struct {
	Global     GlobalConfig `yaml:"global"`
	Categories []Category   `yaml:"categories"`
}

func LoadCategories(path string) (*CategoriesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf CategoriesFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	if cf.Global.Threshold == 0 {
		cf.Global.Threshold = 0.40
	}
	if cf.Global.TopCategories == 0 {
		cf.Global.TopCategories = 2
	}
	for i := range cf.Categories {
		if cf.Categories[i].Threshold == 0 {
			cf.Categories[i].Threshold = cf.Global.Threshold
		}
	}
	return &cf, nil
}
